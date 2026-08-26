package training

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/audit"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/clock"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/dataset"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/outbox"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
)

type Service struct {
	db          *storage.Database
	repo        Repository
	datasets    dataset.Repository
	audits      audit.Store
	outbox      outbox.Repository
	clock       clock.Clock
	leaseTTL    time.Duration
	retryBase   time.Duration
	maxAttempts int
}

type ClaimResult struct {
	Job     domain.TrainingJob
	Attempt domain.JobAttempt
}

func NewService(db *storage.Database, c clock.Clock, leaseTTL, retryBase time.Duration, maxAttempts int) *Service {
	return &Service{db: db, repo: Repository{}, datasets: dataset.Repository{}, audits: audit.Store{}, outbox: outbox.Repository{}, clock: c, leaseTTL: leaseTTL, retryBase: retryBase, maxAttempts: maxAttempts}
}

func (s *Service) Enqueue(ctx context.Context, principal auth.Principal, releaseID, requestID string) (domain.TrainingJob, error) {
	if err := auth.RequireRole(principal, domain.RoleDataSteward); err != nil {
		return domain.TrainingJob{}, err
	}
	if releaseID == "" || requestID == "" {
		return domain.TrainingJob{}, domain.Validation("training.enqueue", "release and request id are required")
	}
	id, err := domain.NewID("job")
	if err != nil {
		return domain.TrainingJob{}, err
	}
	now := s.clock.Now()
	job := domain.TrainingJob{ID: id, TenantID: principal.TenantID, ReleaseID: releaseID, Status: domain.JobQueued, MaxAttempts: s.maxAttempts, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now, Version: 1}
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		release, err := s.datasets.FindRelease(ctx, tx, principal.TenantID, releaseID)
		if err != nil {
			return err
		}
		if release.Status != domain.DatasetStatusPublished {
			return domain.Precondition("training.enqueue", "dataset_release", releaseID, "release must be published")
		}
		if err := s.repo.InsertJob(ctx, tx, job); err != nil {
			return err
		}
		return s.appendEffects(ctx, tx, principal.TenantID, principal.UserID, requestID, "training.enqueue", job.ID, "queued", now)
	})
	if err != nil {
		return domain.TrainingJob{}, fmt.Errorf("enqueue training job: %w", err)
	}
	return job, nil
}

func (s *Service) Claim(ctx context.Context, principal auth.Principal) (ClaimResult, error) {
	if err := auth.RequireRole(principal, domain.RoleWorker); err != nil {
		return ClaimResult{}, err
	}
	token, err := newLeaseToken()
	if err != nil {
		return ClaimResult{}, err
	}
	now := s.clock.Now()
	var result ClaimResult
	err = s.db.Write(ctx, func(tx *sql.Tx) error {
		job, err := s.repo.FindDue(ctx, tx, principal.TenantID, now)
		if err != nil {
			return err
		}
		if job.AttemptCount >= job.MaxAttempts {
			return domain.Precondition("training.claim", "training_job", job.ID, "job exhausted its attempt budget")
		}
		expires := now.Add(s.leaseTTL)
		if err := s.repo.Claim(ctx, tx, job, principal.UserID, token, now, expires); err != nil {
			return err
		}
		attemptID, err := domain.NewID("attempt")
		if err != nil {
			return err
		}
		attempt := domain.JobAttempt{ID: attemptID, TenantID: principal.TenantID, JobID: job.ID, Attempt: job.AttemptCount + 1, WorkerID: principal.UserID, StartedAt: now, Outcome: "running"}
		if err := s.repo.InsertAttempt(ctx, tx, attempt); err != nil {
			return err
		}
		if err := s.appendEffects(ctx, tx, principal.TenantID, principal.UserID, "worker-claim", "training.claim", job.ID, "running", now); err != nil {
			return err
		}
		job.Status, job.Owner, job.LeaseToken, job.LeaseExpiresAt = domain.JobRunning, principal.UserID, token, &expires
		job.AttemptCount, job.UpdatedAt, job.Version = job.AttemptCount+1, now, job.Version+1
		result = ClaimResult{Job: job, Attempt: attempt}
		return nil
	})
	if err != nil {
		return ClaimResult{}, fmt.Errorf("claim training job: %w", err)
	}
	return result, nil
}

func (s *Service) Renew(ctx context.Context, principal auth.Principal, jobID, token string, version int64) (domain.TrainingJob, error) {
	if err := auth.RequireRole(principal, domain.RoleWorker); err != nil {
		return domain.TrainingJob{}, err
	}
	now := s.clock.Now()
	expires := now.Add(s.leaseTTL)
	var renewed domain.TrainingJob
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		job, err := s.repo.FindJob(ctx, tx, principal.TenantID, jobID)
		if err != nil {
			return err
		}
		if job.Owner != principal.UserID || job.LeaseToken != token || job.Version != version {
			return domain.Wrap(domain.ErrLeaseLost, "training.renew", "training_job", jobID, "lease identity or ownership changed", nil)
		}
		if err := s.repo.Renew(ctx, tx, principal.TenantID, jobID, principal.UserID, token, version, now, expires); err != nil {
			return err
		}
		job.LeaseExpiresAt, job.UpdatedAt, job.Version = &expires, now, job.Version+1
		renewed = job
		return nil
	})
	if err != nil {
		return domain.TrainingJob{}, fmt.Errorf("renew training job: %w", err)
	}
	return renewed, nil
}

func (s *Service) Checkpoint(ctx context.Context, principal auth.Principal, jobID, token, checkpoint string, version int64) (domain.TrainingJob, error) {
	if err := auth.RequireRole(principal, domain.RoleWorker); err != nil {
		return domain.TrainingJob{}, err
	}
	checkpoint = strings.TrimSpace(checkpoint)
	if checkpoint == "" || len(checkpoint) > 2048 {
		return domain.TrainingJob{}, domain.Validation("training.checkpoint", "bounded checkpoint is required")
	}
	now := s.clock.Now()
	var updated domain.TrainingJob
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		job, err := s.repo.FindJob(ctx, tx, principal.TenantID, jobID)
		if err != nil {
			return err
		}
		if job.Owner != principal.UserID || job.LeaseToken != token || job.Version != version {
			return domain.Wrap(domain.ErrLeaseLost, "training.checkpoint", "training_job", jobID, "lease identity or ownership changed", nil)
		}
		if err := s.repo.SaveCheckpoint(ctx, tx, principal.TenantID, jobID, principal.UserID, token, checkpoint, version, now); err != nil {
			return err
		}
		job.Checkpoint, job.UpdatedAt, job.Version = checkpoint, now, job.Version+1
		updated = job
		return nil
	})
	if err != nil {
		return domain.TrainingJob{}, fmt.Errorf("save training checkpoint: %w", err)
	}
	return updated, nil
}

func (s *Service) Complete(ctx context.Context, principal auth.Principal, jobID, token, outputURI, requestID string, version int64) (domain.TrainingJob, error) {
	if err := auth.RequireRole(principal, domain.RoleWorker); err != nil {
		return domain.TrainingJob{}, err
	}
	outputURI = strings.TrimSpace(outputURI)
	if outputURI == "" || requestID == "" {
		return domain.TrainingJob{}, domain.Validation("training.complete", "output URI and request id are required")
	}
	now := s.clock.Now()
	var completed domain.TrainingJob
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		job, err := s.repo.FindJob(ctx, tx, principal.TenantID, jobID)
		if err != nil {
			return err
		}
		if job.Owner != principal.UserID || job.LeaseToken != token || job.Version != version {
			return domain.Wrap(domain.ErrLeaseLost, "training.complete", "training_job", jobID, "lease identity or ownership changed", nil)
		}
		if err := job.Status.Transition(domain.JobSucceeded); err != nil {
			return err
		}
		if strings.TrimSpace(job.Checkpoint) == "" {
			return domain.Precondition("training.complete", "training_job", jobID, "durable checkpoint is required before completion")
		}
		if err := s.repo.Complete(ctx, tx, principal.TenantID, jobID, principal.UserID, token, outputURI, version, now); err != nil {
			return err
		}
		if err := s.repo.FinishAttempt(ctx, tx, principal.TenantID, jobID, job.AttemptCount, "succeeded", "", now); err != nil {
			return err
		}
		if err := s.appendEffects(ctx, tx, principal.TenantID, principal.UserID, requestID, "training.complete", jobID, "succeeded", now); err != nil {
			return err
		}
		job.Status, job.Owner, job.LeaseToken, job.LeaseExpiresAt = domain.JobSucceeded, "", "", nil
		job.OutputURI, job.UpdatedAt, job.Version = outputURI, now, job.Version+1
		completed = job
		return nil
	})
	if err != nil {
		return domain.TrainingJob{}, fmt.Errorf("complete training job: %w", err)
	}
	return completed, nil
}

func (s *Service) Fail(ctx context.Context, principal auth.Principal, jobID, token, message, requestID string, version int64, permanent bool) (domain.TrainingJob, error) {
	if err := auth.RequireRole(principal, domain.RoleWorker); err != nil {
		return domain.TrainingJob{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" || requestID == "" {
		return domain.TrainingJob{}, domain.Validation("training.fail", "error message and request id are required")
	}
	now := s.clock.Now()
	var failed domain.TrainingJob
	err := s.db.Write(ctx, func(tx *sql.Tx) error {
		job, err := s.repo.FindJob(ctx, tx, principal.TenantID, jobID)
		if err != nil {
			return err
		}
		if job.Owner != principal.UserID || job.LeaseToken != token || job.Version != version {
			return domain.Wrap(domain.ErrLeaseLost, "training.fail", "training_job", jobID, "lease identity or ownership changed", nil)
		}
		retry := !permanent && job.AttemptCount < job.MaxAttempts
		to := domain.JobFailed
		outcome := "failed"
		nextAttempt := now
		if retry {
			to, outcome = domain.JobRetrying, "retrying"
			nextAttempt = now.Add(s.backoff(job.AttemptCount))
		}
		if err := job.Status.Transition(to); err != nil {
			return err
		}
		if err := s.repo.Fail(ctx, tx, job, principal.UserID, token, message, retry, nextAttempt, now); err != nil {
			return err
		}
		if err := s.repo.FinishAttempt(ctx, tx, principal.TenantID, jobID, job.AttemptCount, outcome, message, now); err != nil {
			return err
		}
		if err := s.appendEffects(ctx, tx, principal.TenantID, principal.UserID, requestID, "training.fail", jobID, outcome, now); err != nil {
			return err
		}
		job.Status, job.Owner, job.LeaseToken, job.LeaseExpiresAt = to, "", "", nil
		job.LastError, job.NextAttemptAt, job.UpdatedAt, job.Version = message, nextAttempt, now, job.Version+1
		failed = job
		return nil
	})
	if err != nil {
		return domain.TrainingJob{}, fmt.Errorf("fail training job: %w", err)
	}
	return failed, nil
}

func (s *Service) Get(ctx context.Context, principal auth.Principal, jobID string) (domain.TrainingJob, error) {
	return s.repo.FindJob(ctx, s.db.SQL(), principal.TenantID, jobID)
}

func (s *Service) backoff(attempt int) time.Duration {
	exponent := math.Min(float64(attempt-1), 8)
	return time.Duration(float64(s.retryBase) * math.Pow(2, exponent))
}

func (s *Service) appendEffects(ctx context.Context, tx *sql.Tx, tenantID, actorID, requestID, action, jobID, outcome string, now time.Time) error {
	auditID, err := domain.NewID("audit")
	if err != nil {
		return err
	}
	eventID, err := domain.NewID("event")
	if err != nil {
		return err
	}
	if err := s.audits.Append(ctx, tx, audit.Record{ID: auditID, TenantID: tenantID, ActorID: actorID, Action: action, ObjectType: "training_job", ObjectID: jobID, Outcome: outcome, RequestID: requestID, CreatedAt: now}); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"job_id": jobID, "event": action, "outcome": outcome})
	return s.outbox.Enqueue(ctx, tx, domain.OutboxEvent{ID: eventID, TenantID: tenantID, Topic: action, AggregateType: "training_job", AggregateID: jobID, Payload: string(payload), Status: domain.OutboxPending, MaxAttempts: 5, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now, Version: 1})
}
