package jobstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hubcr.io/hubcr/internal/modules/jobs"
)

type Store struct{ database *gorm.DB }

func New(database *gorm.DB) *Store { return &Store{database: database} }

func (s *Store) ByID(ctx context.Context, id string) (jobs.Job, error) {
	if id == "" {
		return jobs.Job{}, jobs.ErrInvalidJob
	}
	var record jobRecord
	if err := s.database.WithContext(ctx).Where("id = ?", id).First(&record).Error; err != nil {
		return jobs.Job{}, classify("find job", err)
	}
	job, err := jobFromRecord(record)
	if err != nil {
		return jobs.Job{}, classify("decode job", err)
	}
	return job, nil
}

func (s *Store) Enqueue(
	ctx context.Context,
	intent jobs.Intent,
	createdAt time.Time,
) (jobs.Job, bool, error) {
	if _, err := jobs.NewIntent(
		string(intent.Kind), intent.Key, intent.Payload, intent.MaxAttempts, intent.AvailableAt,
	); err != nil || createdAt.IsZero() {
		return jobs.Job{}, false, jobs.ErrInvalidJob
	}
	id, err := newID()
	if err != nil {
		return jobs.Job{}, false, classify("generate job ID", err)
	}
	createdAt = createdAt.UTC().Round(time.Microsecond)
	record := jobRecord{
		ID: id, Kind: string(intent.Kind), IntentKey: intent.Key,
		Payload: []byte(intent.Payload), State: string(jobs.StateQueued),
		MaxAttempts: intent.MaxAttempts, AvailableAt: intent.AvailableAt.UTC(),
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	result := s.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "intent_key"}},
		DoNothing: true,
	}).Create(&record)
	if result.Error != nil {
		return jobs.Job{}, false, classify("enqueue job", result.Error)
	}
	created := result.RowsAffected == 1

	var current jobRecord
	if err := s.database.WithContext(ctx).Where("intent_key = ?", intent.Key).First(&current).Error; err != nil {
		return jobs.Job{}, false, classify("read enqueued job", err)
	}
	job, err := jobFromRecord(current)
	if err != nil {
		return jobs.Job{}, false, classify("decode enqueued job", err)
	}
	if job.Kind != intent.Kind || job.MaxAttempts != intent.MaxAttempts ||
		!jsonEqual(job.Payload, intent.Payload) {
		return jobs.Job{}, false, fmt.Errorf("enqueue job: %w", jobs.ErrConflict)
	}
	return job, created, nil
}

func (s *Store) Claim(ctx context.Context, claim jobs.Claim) (jobs.Job, error) {
	return s.claim(ctx, claim, nil)
}

func (s *Store) ClaimKinds(
	ctx context.Context,
	claim jobs.Claim,
	kinds []jobs.Kind,
) (jobs.Job, error) {
	if len(kinds) == 0 {
		return jobs.Job{}, jobs.ErrNoJob
	}
	values := make([]string, len(kinds))
	seen := make(map[jobs.Kind]struct{}, len(kinds))
	for index, kind := range kinds {
		validated, err := jobs.ParseKind(string(kind))
		if err != nil {
			return jobs.Job{}, err
		}
		if _, exists := seen[validated]; exists {
			return jobs.Job{}, jobs.ErrInvalidJob
		}
		seen[validated] = struct{}{}
		values[index] = string(validated)
	}
	return s.claim(ctx, claim, values)
}

func (s *Store) claim(ctx context.Context, claim jobs.Claim, kinds []string) (jobs.Job, error) {
	validated, err := jobs.NewClaim(claim.WorkerID, claim.Now, claim.LeaseDuration)
	if err != nil {
		return jobs.Job{}, err
	}
	var claimed jobRecord
	err = s.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		terminal := transaction.Exec(
			`UPDATE jobs
			 SET state = 'DEAD', lease_owner = NULL, lease_expires_at = NULL,
			     last_error_code = 'LEASE_EXPIRED', completed_at = ?, updated_at = ?
			 WHERE state = 'RUNNING' AND lease_expires_at <= ? AND attempt_count >= max_attempts`,
			validated.Now, validated.Now, validated.Now,
		)
		if terminal.Error != nil {
			return terminal.Error
		}

		leaseExpiresAt := validated.Now.Add(validated.LeaseDuration).UTC().Round(time.Microsecond)
		query := `WITH candidate AS (
			    SELECT id
			    FROM jobs
			    WHERE ((state = 'QUEUED' AND available_at <= ?)
			        OR (state = 'RUNNING' AND lease_expires_at <= ?))
			      AND attempt_count < max_attempts
			      %s
			    ORDER BY CASE WHEN state = 'RUNNING' THEN 0 ELSE 1 END,
			             COALESCE(lease_expires_at, available_at), created_at, id
			    FOR UPDATE SKIP LOCKED
			    LIMIT 1
			)
			UPDATE jobs AS job
			SET state = 'RUNNING', attempt_count = job.attempt_count + 1,
			    lease_owner = ?, lease_expires_at = ?,
			    last_error_code = CASE WHEN job.state = 'RUNNING' THEN 'LEASE_EXPIRED' ELSE job.last_error_code END,
			    started_at = COALESCE(job.started_at, ?), completed_at = NULL, updated_at = ?
			FROM candidate
			WHERE job.id = candidate.id
			RETURNING job.*`
		arguments := []any{validated.Now, validated.Now}
		kindFilter := ""
		if len(kinds) > 0 {
			kindFilter = "AND kind IN ?"
			arguments = append(arguments, kinds)
		}
		arguments = append(
			arguments, validated.WorkerID, leaseExpiresAt, validated.Now, validated.Now,
		)
		result := transaction.Raw(fmt.Sprintf(query, kindFilter), arguments...).Scan(&claimed)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return jobs.ErrNoJob
		}
		return nil
	})
	if err != nil {
		return jobs.Job{}, classify("claim job", err)
	}
	job, err := jobFromRecord(claimed)
	if err != nil {
		return jobs.Job{}, classify("decode claimed job", err)
	}
	return job, nil
}

func (s *Store) Complete(ctx context.Context, completion jobs.Completion) error {
	validated, err := jobs.NewCompletion(completion.JobID, completion.WorkerID, completion.Now)
	if err != nil {
		return err
	}
	result := s.database.WithContext(ctx).Model(&jobRecord{}).
		Where(
			"id = ? AND state = 'RUNNING' AND lease_owner = ? AND lease_expires_at > ?",
			validated.JobID, validated.WorkerID, validated.Now,
		).
		Updates(map[string]any{
			"state": string(jobs.StateSucceeded), "lease_owner": nil, "lease_expires_at": nil,
			"completed_at": validated.Now, "updated_at": validated.Now,
		})
	if result.Error != nil {
		return classify("complete job", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("complete job: %w", jobs.ErrLeaseLost)
	}
	return nil
}

func (s *Store) Fail(ctx context.Context, failure jobs.Failure) error {
	validated, err := jobs.NewFailure(
		failure.JobID, failure.WorkerID, string(failure.Code), failure.Now,
		failure.RetryAt, failure.Terminal,
	)
	if err != nil {
		return err
	}
	result := s.database.WithContext(ctx).Exec(
		`UPDATE jobs
		 SET state = CASE WHEN ? OR attempt_count >= max_attempts THEN 'DEAD' ELSE 'QUEUED' END,
		     available_at = ?, lease_owner = NULL, lease_expires_at = NULL,
		     last_error_code = ?,
		     completed_at = CASE WHEN ? OR attempt_count >= max_attempts THEN CAST(? AS timestamptz) ELSE NULL END,
		     updated_at = ?
		 WHERE id = ? AND state = 'RUNNING' AND lease_owner = ? AND lease_expires_at > ?`,
		validated.Terminal, validated.RetryAt, string(validated.Code),
		validated.Terminal, validated.Now, validated.Now,
		validated.JobID, validated.WorkerID, validated.Now,
	)
	if result.Error != nil {
		return classify("fail job", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("fail job: %w", jobs.ErrLeaseLost)
	}
	return nil
}

func jobFromRecord(record jobRecord) (jobs.Job, error) {
	kind, err := jobs.ParseKind(record.Kind)
	if err != nil {
		return jobs.Job{}, err
	}
	state, err := jobs.ParseState(record.State)
	if err != nil {
		return jobs.Job{}, err
	}
	job := jobs.Job{
		ID: record.ID, Kind: kind, IntentKey: record.IntentKey,
		Payload: json.RawMessage(append([]byte(nil), record.Payload...)), State: state,
		Attempts: record.AttemptCount, MaxAttempts: record.MaxAttempts,
		AvailableAt: record.AvailableAt.UTC(), LeaseExpiresAt: cloneTime(record.LeaseExpiresAt),
		CreatedAt: record.CreatedAt.UTC(), UpdatedAt: record.UpdatedAt.UTC(),
		StartedAt: cloneTime(record.StartedAt), CompletedAt: cloneTime(record.CompletedAt),
	}
	if record.LeaseOwner != nil {
		job.LeaseOwner = *record.LeaseOwner
	}
	if record.LastErrorCode != nil {
		code, err := jobs.ParseErrorCode(*record.LastErrorCode)
		if err != nil {
			return jobs.Job{}, err
		}
		job.LastErrorCode = &code
	}
	if err := job.Validate(); err != nil {
		return jobs.Job{}, err
	}
	return job, nil
}

func jsonEqual(first, second []byte) bool {
	var left any
	var right any
	return json.Unmarshal(first, &left) == nil && json.Unmarshal(second, &right) == nil &&
		reflect.DeepEqual(left, right)
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded[:]), nil
}

func classify(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, jobs.ErrInvalidJob) || errors.Is(err, jobs.ErrNoJob) ||
		errors.Is(err, jobs.ErrLeaseLost) || errors.Is(err, jobs.ErrConflict) ||
		errors.Is(err, jobs.ErrUnavailable) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, jobs.ErrNoJob)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return fmt.Errorf("%s: %w", operation, jobs.ErrConflict)
		case "23502", "23514", "22P02":
			return fmt.Errorf("%s: %w", operation, jobs.ErrInvalidJob)
		}
	}
	return fmt.Errorf("%s: %w", operation, jobs.ErrUnavailable)
}

var _ jobs.Store = (*Store)(nil)
