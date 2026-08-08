package jobs

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

const (
	MaxKindLength      = 64
	MaxIntentKeyLength = 255
	MaxPayloadBytes    = 64 * 1024
	MaxAttempts        = 100
	MaxWorkerIDLength  = 128
	MaxErrorCodeLength = 64
)

var (
	ErrInvalidJob  = errors.New("invalid job")
	ErrNoJob       = errors.New("no job is available")
	ErrLeaseLost   = errors.New("job lease is no longer owned")
	ErrConflict    = errors.New("job intent conflicts with persisted state")
	ErrUnavailable = errors.New("job persistence is unavailable")

	identifierPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

type Kind string

func ParseKind(value string) (Kind, error) {
	if len(value) < 1 || len(value) > MaxKindLength || !identifierPattern.MatchString(value) {
		return "", ErrInvalidJob
	}
	return Kind(value), nil
}

type State string

const (
	StateQueued    State = "QUEUED"
	StateRunning   State = "RUNNING"
	StateSucceeded State = "SUCCEEDED"
	StateDead      State = "DEAD"
)

func ParseState(value string) (State, error) {
	switch State(value) {
	case StateQueued, StateRunning, StateSucceeded, StateDead:
		return State(value), nil
	default:
		return "", ErrInvalidJob
	}
}

type ErrorCode string

func ParseErrorCode(value string) (ErrorCode, error) {
	if len(value) < 1 || len(value) > MaxErrorCodeLength || !identifierPattern.MatchString(value) {
		return "", ErrInvalidJob
	}
	return ErrorCode(value), nil
}

type Intent struct {
	Kind        Kind
	Key         string
	Payload     json.RawMessage
	MaxAttempts int
	AvailableAt time.Time
}

func NewIntent(
	rawKind string,
	key string,
	payload json.RawMessage,
	maxAttempts int,
	availableAt time.Time,
) (Intent, error) {
	kind, err := ParseKind(rawKind)
	if err != nil || !validIntentKey(key) || maxAttempts < 1 || maxAttempts > MaxAttempts ||
		availableAt.IsZero() {
		return Intent{}, ErrInvalidJob
	}
	compactPayload, err := normalizePayload(payload)
	if err != nil {
		return Intent{}, err
	}
	return Intent{
		Kind: kind, Key: key, Payload: compactPayload, MaxAttempts: maxAttempts,
		AvailableAt: availableAt.UTC().Round(time.Microsecond),
	}, nil
}

type Job struct {
	ID             string
	Kind           Kind
	IntentKey      string
	Payload        json.RawMessage
	State          State
	Attempts       int
	MaxAttempts    int
	AvailableAt    time.Time
	LeaseOwner     string
	LeaseExpiresAt *time.Time
	LastErrorCode  *ErrorCode
	CreatedAt      time.Time
	UpdatedAt      time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
}

func (j Job) Validate() error {
	if j.ID == "" || !validIntentKey(j.IntentKey) || j.Attempts < 0 ||
		j.MaxAttempts < 1 || j.MaxAttempts > MaxAttempts || j.Attempts > j.MaxAttempts ||
		j.AvailableAt.IsZero() || j.CreatedAt.IsZero() || j.UpdatedAt.IsZero() ||
		j.UpdatedAt.Before(j.CreatedAt) {
		return ErrInvalidJob
	}
	if _, err := ParseKind(string(j.Kind)); err != nil {
		return err
	}
	if _, err := ParseState(string(j.State)); err != nil {
		return err
	}
	if _, err := normalizePayload(j.Payload); err != nil {
		return err
	}
	if j.LastErrorCode != nil {
		if _, err := ParseErrorCode(string(*j.LastErrorCode)); err != nil {
			return err
		}
	}
	if j.StartedAt != nil && j.StartedAt.IsZero() || j.CompletedAt != nil && j.CompletedAt.IsZero() ||
		j.LeaseExpiresAt != nil && j.LeaseExpiresAt.IsZero() {
		return ErrInvalidJob
	}
	switch j.State {
	case StateQueued:
		if j.LeaseOwner != "" || j.LeaseExpiresAt != nil || j.CompletedAt != nil {
			return ErrInvalidJob
		}
	case StateRunning:
		if !validWorkerID(j.LeaseOwner) || j.LeaseExpiresAt == nil || j.StartedAt == nil ||
			j.CompletedAt != nil || j.Attempts < 1 {
			return ErrInvalidJob
		}
	case StateSucceeded, StateDead:
		if j.LeaseOwner != "" || j.LeaseExpiresAt != nil || j.CompletedAt == nil {
			return ErrInvalidJob
		}
	}
	return nil
}

type Claim struct {
	WorkerID      string
	Now           time.Time
	LeaseDuration time.Duration
}

func NewClaim(workerID string, now time.Time, leaseDuration time.Duration) (Claim, error) {
	if !validWorkerID(workerID) || now.IsZero() || leaseDuration <= 0 {
		return Claim{}, ErrInvalidJob
	}
	return Claim{
		WorkerID:      workerID,
		Now:           now.UTC().Round(time.Microsecond),
		LeaseDuration: leaseDuration,
	}, nil
}

type Completion struct {
	JobID    string
	WorkerID string
	Now      time.Time
}

func NewCompletion(jobID, workerID string, now time.Time) (Completion, error) {
	if jobID == "" || !validWorkerID(workerID) || now.IsZero() {
		return Completion{}, ErrInvalidJob
	}
	return Completion{JobID: jobID, WorkerID: workerID, Now: now.UTC().Round(time.Microsecond)}, nil
}

type Failure struct {
	Completion
	Code     ErrorCode
	RetryAt  time.Time
	Terminal bool
}

func NewFailure(
	jobID string,
	workerID string,
	rawCode string,
	now time.Time,
	retryAt time.Time,
	terminal bool,
) (Failure, error) {
	completion, err := NewCompletion(jobID, workerID, now)
	if err != nil {
		return Failure{}, err
	}
	code, err := ParseErrorCode(rawCode)
	if err != nil || retryAt.IsZero() || retryAt.Before(completion.Now) {
		return Failure{}, ErrInvalidJob
	}
	return Failure{
		Completion: completion,
		Code:       code,
		RetryAt:    retryAt.UTC().Round(time.Microsecond),
		Terminal:   terminal,
	}, nil
}

func validIntentKey(value string) bool {
	if len(value) < 1 || len(value) > MaxIntentKeyLength {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func validWorkerID(value string) bool {
	if len(value) < 1 || len(value) > MaxWorkerIDLength || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func normalizePayload(payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) < 2 || len(payload) > MaxPayloadBytes || !json.Valid(payload) {
		return nil, ErrInvalidJob
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, ErrInvalidJob
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, ErrInvalidJob
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err != nil {
		return nil, ErrInvalidJob
	}
	return json.RawMessage(bytes.Clone(compact.Bytes())), nil
}
