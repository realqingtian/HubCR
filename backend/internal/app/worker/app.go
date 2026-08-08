package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"hubcr.io/hubcr/internal/modules/jobs"
	"hubcr.io/hubcr/internal/modules/registry"
	"hubcr.io/hubcr/internal/modules/security"
	"hubcr.io/hubcr/internal/platform/config"
	"hubcr.io/hubcr/internal/platform/postgres"
	"hubcr.io/hubcr/internal/platform/postgres/jobstore"
	"hubcr.io/hubcr/internal/platform/postgres/securitystore"
	cosignverifier "hubcr.io/hubcr/internal/platform/scanner/cosign"
	trivyscanner "hubcr.io/hubcr/internal/platform/scanner/trivy"
)

type Queue interface {
	ClaimKinds(context.Context, string, time.Duration, []jobs.Kind) (jobs.Job, error)
	Complete(context.Context, jobs.Job, string) error
	Fail(context.Context, jobs.Job, string, string, time.Duration, bool) error
}

type databaseCloser interface{ Close() }

type workflowRepairer interface {
	RepairMissingWorkflows(context.Context, int) (int, error)
}

type combinedSecurityRepairer struct {
	scans      *security.Service
	signatures *security.TrustService
}

func (r *combinedSecurityRepairer) RepairMissingWorkflows(
	ctx context.Context,
	limit int,
) (int, error) {
	scans, err := r.scans.RepairMissingWorkflows(ctx, limit)
	if err != nil {
		return scans, err
	}
	signatures, err := r.signatures.RepairMissingVerificationWorkflows(ctx, limit)
	return scans + signatures, err
}

// App composes the durable job queue and bounded asynchronous handlers.
type App struct {
	config   config.Worker
	logger   *slog.Logger
	database databaseCloser
	queue    Queue
	workerID string
	handlers map[jobs.Kind]jobs.Handler
	kinds    []jobs.Kind
	repairer workflowRepairer
}

func New(ctx context.Context, cfg config.Worker, logger *slog.Logger) (*App, error) {
	if logger == nil {
		return nil, errors.New("worker logger must be configured")
	}
	database, err := postgres.Open(ctx, postgres.Options{
		URL:            cfg.Database.URL,
		ConnectTimeout: cfg.Database.ConnectTimeout,
		MaxConnections: cfg.Database.MaxConnections,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize worker PostgreSQL: %w", err)
	}
	if err := database.Check(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("check worker PostgreSQL: %w", err)
	}
	service, err := jobs.NewService(jobstore.New(database.ORM()), time.Now)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize job service: %w", err)
	}
	workerID, err := newWorkerID()
	if err != nil {
		database.Close()
		return nil, errors.New("generate worker identity")
	}
	var handlers map[jobs.Kind]jobs.Handler
	var repairer workflowRepairer
	if cfg.Scanner.Enabled {
		securityStore := securitystore.New(database.ORM())
		securityService, err := security.NewService(securityStore, time.Now)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("initialize security service: %w", err)
		}
		trustService, err := security.NewTrustService(securityStore, time.Now)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("initialize trust service: %w", err)
		}
		privateKeyPEM, err := os.ReadFile(cfg.Scanner.RegistryPrivateKey)
		if err != nil {
			database.Close()
			return nil, errors.New("initialize worker Registry signing key: file is unreadable")
		}
		privateKey, err := registry.ParseRSAPrivateKeyPEM(privateKeyPEM)
		clear(privateKeyPEM)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("initialize worker Registry signing key: %w", err)
		}
		signer, err := registry.NewRS256Signer(privateKey, rand.Reader)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("initialize worker Registry token signer: %w", err)
		}
		tokens, err := registry.NewSystemTokenService(signer, registry.SystemTokenOptions{
			Service: cfg.Scanner.RegistryService, Issuer: cfg.Scanner.RegistryIssuer,
			Subject: "hubcr-security-worker", TokenTTL: cfg.Scanner.RegistryTokenTTL,
			ClockSkew: cfg.Scanner.RegistryClockSkew, Clock: time.Now, Random: rand.Reader,
		})
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("initialize worker Registry token service: %w", err)
		}
		scanner, err := trivyscanner.New(trivyscanner.Options{
			Binary: cfg.Scanner.Binary, CacheDir: cfg.Scanner.CacheDir,
			Insecure: cfg.Scanner.RegistryInsecure, Clock: time.Now,
		})
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("initialize Trivy scanner: %w", err)
		}
		if err := os.MkdirAll(cfg.Scanner.CosignScratchDir, 0o700); err != nil {
			database.Close()
			return nil, errors.New("initialize Cosign scratch directory")
		}
		verifier, err := cosignverifier.New(cosignverifier.Options{
			Binary: cfg.Scanner.CosignBinary, ScratchDir: cfg.Scanner.CosignScratchDir,
			Insecure: cfg.Scanner.RegistryInsecure,
		})
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("initialize Cosign verifier: %w", err)
		}
		securityHandlers, err := security.NewHandlers(
			securityService, scanner, tokens,
			security.HandlerOptions{RegistryHost: cfg.Scanner.RegistryHost, Clock: time.Now},
		)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("initialize security handlers: %w", err)
		}
		handlers = securityHandlers.JobHandlers()
		trustHandlers, err := security.NewTrustHandlers(
			trustService, verifier, tokens,
			security.HandlerOptions{RegistryHost: cfg.Scanner.RegistryHost, Clock: time.Now},
		)
		if err != nil {
			database.Close()
			return nil, fmt.Errorf("initialize trust handlers: %w", err)
		}
		for kind, handler := range trustHandlers.JobHandlers() {
			handlers[kind] = handler
		}
		repairer = &combinedSecurityRepairer{scans: securityService, signatures: trustService}
	}
	app, err := newApp(cfg, logger, database, service, workerID, handlers)
	if err != nil {
		database.Close()
		return nil, err
	}
	app.repairer = repairer
	return app, nil
}

func newApp(
	cfg config.Worker,
	logger *slog.Logger,
	database databaseCloser,
	queue Queue,
	workerID string,
	handlers map[jobs.Kind]jobs.Handler,
) (*App, error) {
	if logger == nil || queue == nil || cfg.MaxConcurrency < 1 || cfg.MaxConcurrency > 64 ||
		cfg.PollInterval <= 0 || cfg.LeaseDuration <= cfg.JobTimeout || cfg.JobTimeout <= 0 ||
		cfg.ShutdownTimeout <= 0 || cfg.RetryBase <= 0 || cfg.RetryMax < cfg.RetryBase ||
		cfg.Database.ConnectTimeout <= 0 {
		return nil, errors.New("worker dependencies and execution bounds must be configured")
	}
	if _, err := jobs.NewClaim(workerID, time.Now(), time.Second); err != nil {
		return nil, errors.New("worker identity must be valid")
	}
	if handlers == nil {
		handlers = map[jobs.Kind]jobs.Handler{}
	}
	for kind, handler := range handlers {
		if _, err := jobs.ParseKind(string(kind)); err != nil || handler == nil {
			return nil, errors.New("worker handlers must have valid kinds and implementations")
		}
	}
	kinds := make([]jobs.Kind, 0, len(handlers))
	for kind := range handlers {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return &App{
		config: cfg, logger: logger, database: database, queue: queue,
		workerID: workerID, handlers: handlers, kinds: kinds,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	if a.database != nil {
		defer a.database.Close()
	}
	done := make(chan struct{}, int(a.config.MaxConcurrency))
	active := 0
	a.logger.Info(
		"worker started",
		"worker_id", a.workerID,
		"poll_interval", a.config.PollInterval,
		"max_concurrency", a.config.MaxConcurrency,
	)
	nextRepair := time.Time{}

	for {
		if a.repairer != nil && (nextRepair.IsZero() || !time.Now().Before(nextRepair)) && ctx.Err() == nil {
			repaired, err := a.repairer.RepairMissingWorkflows(ctx, int(a.config.Scanner.RepairBatch))
			if err != nil {
				a.logger.Warn("security workflow repair failed", "error_class", "UNAVAILABLE")
			} else if repaired > 0 {
				a.logger.Info("security workflows repaired", "count", repaired)
			}
			nextRepair = time.Now().Add(a.config.Scanner.RepairInterval)
		}
		for len(a.handlers) > 0 && active < int(a.config.MaxConcurrency) && ctx.Err() == nil {
			job, err := a.queue.ClaimKinds(ctx, a.workerID, a.config.LeaseDuration, a.kinds)
			if errors.Is(err, jobs.ErrNoJob) {
				break
			}
			if err != nil {
				a.logger.Error("job claim failed", "error_class", persistenceErrorClass(err))
				break
			}
			active++
			go func() {
				defer func() { done <- struct{}{} }()
				a.execute(ctx, job)
			}()
		}

		if ctx.Err() != nil {
			return a.shutdown(active, done)
		}
		timer := time.NewTimer(a.config.PollInterval)
		select {
		case <-ctx.Done():
			stopTimer(timer)
			return a.shutdown(active, done)
		case <-done:
			active--
			stopTimer(timer)
		case <-timer.C:
		}
	}
}

func stopTimer(timer *time.Timer) {
	if timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

func (a *App) execute(ctx context.Context, job jobs.Job) {
	handler, exists := a.handlers[job.Kind]
	jobContext, cancel := context.WithTimeout(ctx, a.config.JobTimeout)
	defer cancel()

	var handlerError error
	if !exists {
		handlerError = jobs.Permanent("HANDLER_UNAVAILABLE", errors.New("job handler is not registered"))
	} else {
		handlerError = handler.Handle(jobContext, job)
	}
	if ctx.Err() != nil {
		a.logger.Info(
			"job execution deferred for lease recovery",
			"job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts,
		)
		return
	}

	operationContext, operationCancel := context.WithTimeout(ctx, a.config.Database.ConnectTimeout)
	defer operationCancel()
	if handlerError == nil && jobContext.Err() == nil {
		if err := a.queue.Complete(operationContext, job, a.workerID); err != nil {
			a.logger.Warn(
				"job completion was not persisted",
				"job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts,
				"error_class", persistenceErrorClass(err),
			)
			return
		}
		a.logger.Info("job completed", "job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts)
		return
	}

	code := jobs.ErrorCode("JOB_TIMEOUT")
	terminal := false
	if jobContext.Err() == nil {
		code, terminal = jobs.ClassifyHandlerError(handlerError)
	}
	backoff := a.retryBackoff(job.Attempts)
	if err := a.queue.Fail(operationContext, job, a.workerID, string(code), backoff, terminal); err != nil {
		a.logger.Warn(
			"job failure was not persisted",
			"job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts,
			"error_class", persistenceErrorClass(err),
		)
		return
	}
	a.logger.Warn(
		"job failed",
		"job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts,
		"error_class", code, "terminal", terminal,
	)
}

func (a *App) shutdown(active int, done <-chan struct{}) error {
	if active == 0 {
		a.logger.Info("worker stopped", "active_jobs", 0)
		return nil
	}
	timer := time.NewTimer(a.config.ShutdownTimeout)
	defer timer.Stop()
	for active > 0 {
		select {
		case <-done:
			active--
		case <-timer.C:
			a.logger.Warn("worker shutdown timeout", "active_jobs", active)
			return nil
		}
	}
	a.logger.Info("worker stopped", "active_jobs", 0)
	return nil
}

func (a *App) retryBackoff(attempt int) time.Duration {
	backoff := a.config.RetryBase
	for current := 1; current < attempt && backoff < a.config.RetryMax; current++ {
		if backoff > a.config.RetryMax/2 {
			return a.config.RetryMax
		}
		backoff *= 2
	}
	if backoff > a.config.RetryMax {
		return a.config.RetryMax
	}
	return backoff
}

func persistenceErrorClass(err error) string {
	switch {
	case errors.Is(err, jobs.ErrLeaseLost):
		return "LEASE_LOST"
	case errors.Is(err, jobs.ErrConflict):
		return "CONFLICT"
	case errors.Is(err, jobs.ErrInvalidJob):
		return "INVALID"
	case errors.Is(err, context.Canceled):
		return "CANCELED"
	case errors.Is(err, context.DeadlineExceeded):
		return "TIMEOUT"
	default:
		return "UNAVAILABLE"
	}
}

func newWorkerID() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "worker-" + hex.EncodeToString(value[:]), nil
}
