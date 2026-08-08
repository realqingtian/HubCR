package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hubcr.io/hubcr/internal/modules/artifacts"
	"hubcr.io/hubcr/internal/modules/security"
)

const (
	NotificationEventsMediaType = "application/vnd.docker.distribution.events.v2+json"
	NotificationActionPush      = "push"
	MaxNotificationEvents       = 100

	OCIImageManifestMediaType    = "application/vnd.oci.image.manifest.v1+json"
	OCIImageIndexMediaType       = "application/vnd.oci.image.index.v1+json"
	DockerImageManifestMediaType = "application/vnd.docker.distribution.manifest.v2+json"
	DockerManifestListMediaType  = "application/vnd.docker.distribution.manifest.list.v2+json"
)

var (
	ErrInvalidNotification     = errors.New("invalid Registry notification")
	ErrNotificationConflict    = errors.New("Registry notification conflicts with persisted metadata")
	ErrNotificationUnavailable = errors.New("Registry notification reconciliation unavailable")
)

type NotificationEnvelope struct {
	Events []NotificationEvent `json:"events"`
}

type NotificationEvent struct {
	ID        string             `json:"id,omitempty"`
	Timestamp time.Time          `json:"timestamp"`
	Action    string             `json:"action,omitempty"`
	Target    NotificationTarget `json:"target"`
}

type NotificationTarget struct {
	MediaType  string                   `json:"mediaType,omitempty"`
	Digest     string                   `json:"digest,omitempty"`
	SizeBytes  *int64                   `json:"size,omitempty"`
	Repository string                   `json:"repository,omitempty"`
	Tag        string                   `json:"tag,omitempty"`
	References []NotificationDescriptor `json:"references,omitempty"`
}

type NotificationDescriptor struct {
	MediaType string                `json:"mediaType,omitempty"`
	Digest    string                `json:"digest,omitempty"`
	SizeBytes *int64                `json:"size,omitempty"`
	Platform  *NotificationPlatform `json:"platform,omitempty"`
}

type NotificationPlatform struct {
	OS           string `json:"os,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	Variant      string `json:"variant,omitempty"`
}

type NotificationResult struct {
	Processed int
	Ignored   int
}

type ArtifactReconciler interface {
	ReconcileArtifact(context.Context, artifacts.Observation) (artifacts.Snapshot, error)
}

type SecurityWorkflowScheduler interface {
	EnsureWorkflow(context.Context, security.Target) (security.Workflow, bool, error)
}

type NotificationService struct {
	repositories RepositoryResolver
	artifacts    ArtifactReconciler
	security     SecurityWorkflowScheduler
}

func NewNotificationService(
	repositories RepositoryResolver,
	artifactReconciler ArtifactReconciler,
	securityScheduler SecurityWorkflowScheduler,
) (*NotificationService, error) {
	if repositories == nil || artifactReconciler == nil || securityScheduler == nil {
		return nil, errors.New("Registry notification service dependencies must be configured")
	}
	return &NotificationService{
		repositories: repositories, artifacts: artifactReconciler, security: securityScheduler,
	}, nil
}

func (s *NotificationService) Process(
	ctx context.Context,
	envelope NotificationEnvelope,
) (NotificationResult, error) {
	if len(envelope.Events) < 1 || len(envelope.Events) > MaxNotificationEvents {
		return NotificationResult{}, ErrInvalidNotification
	}
	result := NotificationResult{}
	for _, event := range envelope.Events {
		observation, namespace, repository, relevant, err := observationFromNotification(event)
		if err != nil {
			return NotificationResult{}, err
		}
		if !relevant {
			result.Ignored++
			continue
		}
		observation.RepositoryID = "notification-validation"
		if _, err := artifacts.NormalizeObservation(observation); err != nil {
			return NotificationResult{}, fmt.Errorf("%w: %v", ErrInvalidNotification, err)
		}
		repositoryContext, err := s.repositories.AuthorizationContext(ctx, "", namespace, repository)
		if err != nil || repositoryContext.Repository.ID == "" {
			return NotificationResult{}, fmt.Errorf(
				"%w: resolve notified repository", ErrNotificationUnavailable,
			)
		}
		observation.RepositoryID = repositoryContext.Repository.ID
		if _, err := s.artifacts.ReconcileArtifact(ctx, observation); err != nil {
			return NotificationResult{}, classifyNotificationArtifactError(err)
		}
		target, err := security.NewTarget(
			repositoryContext.Repository.ID, namespace, repository, observation.Digest,
		)
		if err != nil {
			return NotificationResult{}, fmt.Errorf("%w: construct security target", ErrInvalidNotification)
		}
		if _, _, err := s.security.EnsureWorkflow(ctx, target); err != nil {
			return NotificationResult{}, fmt.Errorf(
				"%w: persist security workflow", ErrNotificationUnavailable,
			)
		}
		result.Processed++
	}
	return result, nil
}

func observationFromNotification(
	event NotificationEvent,
) (artifacts.Observation, string, string, bool, error) {
	kind, relevant := notificationArtifactKind(event.Action, event.Target.MediaType)
	if !relevant {
		return artifacts.Observation{}, "", "", false, nil
	}
	if event.Timestamp.IsZero() {
		return artifacts.Observation{}, "", "", false, ErrInvalidNotification
	}
	namespace, repository, valid := splitNotificationRepository(event.Target.Repository)
	if !valid {
		return artifacts.Observation{}, "", "", false, ErrInvalidNotification
	}
	mediaType := event.Target.MediaType
	observation := artifacts.Observation{
		Digest: event.Target.Digest, Kind: string(kind), MediaType: &mediaType,
		SizeBytes: cloneNotificationSize(event.Target.SizeBytes), ObservedAt: event.Timestamp,
	}
	if event.Target.Tag != "" {
		tag := event.Target.Tag
		observation.Tag = &tag
	}
	if kind == artifacts.KindIndex && len(event.Target.References) > 0 {
		items := make([]artifacts.DescriptorObservation, 0, len(event.Target.References))
		for _, reference := range event.Target.References {
			item := artifacts.DescriptorObservation{
				Digest: reference.Digest, SizeBytes: cloneNotificationSize(reference.SizeBytes),
			}
			if reference.MediaType != "" {
				mediaType := reference.MediaType
				item.MediaType = &mediaType
			}
			if reference.Platform != nil {
				item.Platform = &artifacts.Platform{
					OS: reference.Platform.OS, Architecture: reference.Platform.Architecture,
					Variant: reference.Platform.Variant,
				}
			}
			items = append(items, item)
		}
		observation.Descriptors = &artifacts.DescriptorSetObservation{Items: items}
	}
	return observation, namespace, repository, true, nil
}

func notificationArtifactKind(action, mediaType string) (artifacts.Kind, bool) {
	if action != NotificationActionPush {
		return "", false
	}
	switch mediaType {
	case OCIImageManifestMediaType, DockerImageManifestMediaType:
		return artifacts.KindManifest, true
	case OCIImageIndexMediaType, DockerManifestListMediaType:
		return artifacts.KindIndex, true
	default:
		return "", false
	}
}

func splitNotificationRepository(value string) (string, string, bool) {
	namespace, repository, found := strings.Cut(value, "/")
	if !found || namespace == "" || repository == "" || strings.Contains(repository, "/") {
		return "", "", false
	}
	return namespace, repository, true
}

func cloneNotificationSize(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func classifyNotificationArtifactError(err error) error {
	switch {
	case errors.Is(err, artifacts.ErrConflict):
		return fmt.Errorf("%w: artifact facts differ", ErrNotificationConflict)
	case errors.Is(err, artifacts.ErrInvalidDigest), errors.Is(err, artifacts.ErrInvalidTag),
		errors.Is(err, artifacts.ErrInvalidArtifact):
		return fmt.Errorf("%w: artifact payload is invalid", ErrInvalidNotification)
	default:
		return fmt.Errorf("%w: persist artifact notification", ErrNotificationUnavailable)
	}
}
