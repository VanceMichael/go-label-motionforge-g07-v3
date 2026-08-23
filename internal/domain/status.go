package domain

import "fmt"

type CaptureStatus string

const (
	CapturePlanned    CaptureStatus = "planned"
	CaptureReady      CaptureStatus = "ready"
	CaptureRecording  CaptureStatus = "recording"
	CaptureProcessing CaptureStatus = "processing"
	CaptureValidated  CaptureStatus = "validated"
	CaptureRejected   CaptureStatus = "rejected"
	CaptureCanceled   CaptureStatus = "canceled"
	CaptureArchived   CaptureStatus = "archived"
)

var captureTransitions = map[CaptureStatus]map[CaptureStatus]bool{
	CapturePlanned:    {CaptureReady: true, CaptureCanceled: true},
	CaptureReady:      {CaptureRecording: true, CaptureCanceled: true},
	CaptureRecording:  {CaptureProcessing: true, CaptureCanceled: true},
	CaptureProcessing: {CaptureValidated: true, CaptureRejected: true},
	CaptureRejected:   {CaptureReady: true, CaptureArchived: true},
	CaptureValidated:  {CaptureArchived: true},
	CaptureCanceled:   {CaptureArchived: true},
}

func (s CaptureStatus) Transition(to CaptureStatus) error {
	if captureTransitions[s][to] {
		return nil
	}
	return Precondition("capture.transition", "capture", "", fmt.Sprintf("cannot transition from %s to %s", s, to))
}

type ManifestStatus string

const (
	ManifestOpen    ManifestStatus = "open"
	ManifestSealed  ManifestStatus = "sealed"
	ManifestAligned ManifestStatus = "aligned"
	ManifestInvalid ManifestStatus = "invalid"
)

func (s ManifestStatus) Transition(to ManifestStatus) error {
	valid := (s == ManifestOpen && to == ManifestSealed) ||
		(s == ManifestSealed && (to == ManifestAligned || to == ManifestInvalid)) ||
		(s == ManifestInvalid && to == ManifestOpen)
	if !valid {
		return Precondition("manifest.transition", "manifest", "", fmt.Sprintf("cannot transition from %s to %s", s, to))
	}
	return nil
}

type AnnotationStatus string

const (
	AnnotationOpen      AnnotationStatus = "open"
	AnnotationClaimed   AnnotationStatus = "claimed"
	AnnotationSubmitted AnnotationStatus = "submitted"
	AnnotationAccepted  AnnotationStatus = "accepted"
	AnnotationRework    AnnotationStatus = "rework"
	AnnotationCanceled  AnnotationStatus = "canceled"
)

func (s AnnotationStatus) Transition(to AnnotationStatus) error {
	allowed := map[AnnotationStatus]map[AnnotationStatus]bool{
		AnnotationOpen:      {AnnotationClaimed: true, AnnotationCanceled: true},
		AnnotationClaimed:   {AnnotationSubmitted: true, AnnotationOpen: true},
		AnnotationSubmitted: {AnnotationAccepted: true, AnnotationRework: true},
		AnnotationRework:    {AnnotationClaimed: true, AnnotationCanceled: true},
	}
	if !allowed[s][to] {
		return Precondition("annotation.transition", "annotation_batch", "", fmt.Sprintf("cannot transition from %s to %s", s, to))
	}
	return nil
}

type DatasetStatus string

const (
	DatasetStatusDraft     DatasetStatus = "draft"
	DatasetStatusFrozen    DatasetStatus = "frozen"
	DatasetStatusApproved  DatasetStatus = "approved"
	DatasetStatusPublished DatasetStatus = "published"
	DatasetStatusRevoked   DatasetStatus = "revoked"
	DatasetStatusArchived  DatasetStatus = "archived"
)

func (s DatasetStatus) Transition(to DatasetStatus) error {
	allowed := map[DatasetStatus]map[DatasetStatus]bool{
		DatasetStatusDraft:     {DatasetStatusFrozen: true, DatasetStatusArchived: true},
		DatasetStatusFrozen:    {DatasetStatusApproved: true, DatasetStatusDraft: true},
		DatasetStatusApproved:  {DatasetStatusPublished: true, DatasetStatusDraft: true},
		DatasetStatusPublished: {DatasetStatusRevoked: true},
		DatasetStatusRevoked:   {DatasetStatusArchived: true},
	}
	if !allowed[s][to] {
		return Precondition("dataset.transition", "dataset", "", fmt.Sprintf("cannot transition from %s to %s", s, to))
	}
	return nil
}

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobRetrying  JobStatus = "retrying"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
)

func (s JobStatus) Transition(to JobStatus) error {
	allowed := map[JobStatus]map[JobStatus]bool{
		JobQueued:   {JobRunning: true, JobCanceled: true},
		JobRunning:  {JobSucceeded: true, JobRetrying: true, JobFailed: true, JobCanceled: true},
		JobRetrying: {JobRunning: true, JobFailed: true, JobCanceled: true},
	}
	if !allowed[s][to] {
		return Precondition("job.transition", "training_job", "", fmt.Sprintf("cannot transition from %s to %s", s, to))
	}
	return nil
}

type OutboxStatus string

const (
	OutboxPending    OutboxStatus = "pending"
	OutboxDelivering OutboxStatus = "delivering"
	OutboxDelivered  OutboxStatus = "delivered"
	OutboxDead       OutboxStatus = "dead"
)
