package domain

import (
	"errors"
	"testing"
)

func TestCaptureTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		from CaptureStatus
		to   CaptureStatus
		ok   bool
	}{
		{name: "planned ready", from: CapturePlanned, to: CaptureReady, ok: true},
		{name: "planned canceled", from: CapturePlanned, to: CaptureCanceled, ok: true},
		{name: "ready recording", from: CaptureReady, to: CaptureRecording, ok: true},
		{name: "ready canceled", from: CaptureReady, to: CaptureCanceled, ok: true},
		{name: "recording processing", from: CaptureRecording, to: CaptureProcessing, ok: true},
		{name: "recording canceled", from: CaptureRecording, to: CaptureCanceled, ok: true},
		{name: "processing validated", from: CaptureProcessing, to: CaptureValidated, ok: true},
		{name: "processing rejected", from: CaptureProcessing, to: CaptureRejected, ok: true},
		{name: "rejected ready", from: CaptureRejected, to: CaptureReady, ok: true},
		{name: "rejected archived", from: CaptureRejected, to: CaptureArchived, ok: true},
		{name: "validated archived", from: CaptureValidated, to: CaptureArchived, ok: true},
		{name: "canceled archived", from: CaptureCanceled, to: CaptureArchived, ok: true},
		{name: "planned processing", from: CapturePlanned, to: CaptureProcessing},
		{name: "ready validated", from: CaptureReady, to: CaptureValidated},
		{name: "recording ready", from: CaptureRecording, to: CaptureReady},
		{name: "processing recording", from: CaptureProcessing, to: CaptureRecording},
		{name: "validated rejected", from: CaptureValidated, to: CaptureRejected},
		{name: "archived ready", from: CaptureArchived, to: CaptureReady},
		{name: "unknown ready", from: CaptureStatus("unknown"), to: CaptureReady},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.from.Transition(test.to)
			if test.ok && err != nil {
				t.Fatalf("expected transition to succeed: %v", err)
			}
			if !test.ok && !errors.Is(err, ErrPrecondition) {
				t.Fatalf("expected precondition error, got %v", err)
			}
		})
	}
}

func TestManifestTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		from ManifestStatus
		to   ManifestStatus
		ok   bool
	}{
		{ManifestOpen, ManifestSealed, true},
		{ManifestSealed, ManifestAligned, true},
		{ManifestSealed, ManifestInvalid, true},
		{ManifestInvalid, ManifestOpen, true},
		{ManifestOpen, ManifestAligned, false},
		{ManifestAligned, ManifestOpen, false},
		{ManifestInvalid, ManifestAligned, false},
	}
	for _, test := range tests {
		err := test.from.Transition(test.to)
		if test.ok && err != nil {
			t.Errorf("%s -> %s unexpectedly failed: %v", test.from, test.to, err)
		}
		if !test.ok && !errors.Is(err, ErrPrecondition) {
			t.Errorf("%s -> %s should be rejected, got %v", test.from, test.to, err)
		}
	}
}

func TestAnnotationTransitions(t *testing.T) {
	t.Parallel()
	allowed := map[AnnotationStatus][]AnnotationStatus{
		AnnotationOpen:      {AnnotationClaimed, AnnotationCanceled},
		AnnotationClaimed:   {AnnotationSubmitted, AnnotationOpen},
		AnnotationSubmitted: {AnnotationAccepted, AnnotationRework},
		AnnotationRework:    {AnnotationClaimed, AnnotationCanceled},
	}
	states := []AnnotationStatus{AnnotationOpen, AnnotationClaimed, AnnotationSubmitted, AnnotationAccepted, AnnotationRework, AnnotationCanceled}
	for _, from := range states {
		for _, to := range states {
			want := false
			for _, candidate := range allowed[from] {
				if candidate == to {
					want = true
				}
			}
			err := from.Transition(to)
			if want && err != nil {
				t.Errorf("%s -> %s unexpectedly failed: %v", from, to, err)
			}
			if !want && !errors.Is(err, ErrPrecondition) {
				t.Errorf("%s -> %s should fail with precondition, got %v", from, to, err)
			}
		}
	}
}

func TestDatasetTransitions(t *testing.T) {
	t.Parallel()
	allowed := map[DatasetStatus][]DatasetStatus{
		DatasetStatusDraft:     {DatasetStatusFrozen, DatasetStatusArchived},
		DatasetStatusFrozen:    {DatasetStatusApproved, DatasetStatusDraft},
		DatasetStatusApproved:  {DatasetStatusPublished, DatasetStatusDraft},
		DatasetStatusPublished: {DatasetStatusRevoked},
		DatasetStatusRevoked:   {DatasetStatusArchived},
	}
	states := []DatasetStatus{DatasetStatusDraft, DatasetStatusFrozen, DatasetStatusApproved, DatasetStatusPublished, DatasetStatusRevoked, DatasetStatusArchived}
	for _, from := range states {
		for _, to := range states {
			want := false
			for _, candidate := range allowed[from] {
				want = want || candidate == to
			}
			err := from.Transition(to)
			if want != (err == nil) {
				t.Errorf("transition %s -> %s: wanted success=%v, got %v", from, to, want, err)
			}
		}
	}
}

func TestJobTransitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		from JobStatus
		to   JobStatus
		ok   bool
	}{
		{JobQueued, JobRunning, true},
		{JobQueued, JobCanceled, true},
		{JobRunning, JobSucceeded, true},
		{JobRunning, JobRetrying, true},
		{JobRunning, JobFailed, true},
		{JobRunning, JobCanceled, true},
		{JobRetrying, JobRunning, true},
		{JobRetrying, JobFailed, true},
		{JobRetrying, JobCanceled, true},
		{JobQueued, JobSucceeded, false},
		{JobSucceeded, JobRunning, false},
		{JobFailed, JobRetrying, false},
	}
	for _, test := range tests {
		err := test.from.Transition(test.to)
		if test.ok && err != nil {
			t.Errorf("expected %s -> %s to succeed: %v", test.from, test.to, err)
		}
		if !test.ok && !errors.Is(err, ErrPrecondition) {
			t.Errorf("expected %s -> %s to be rejected, got %v", test.from, test.to, err)
		}
	}
}

func TestRolesAndStreamKinds(t *testing.T) {
	t.Parallel()
	roles := []struct {
		value Role
		valid bool
	}{
		{RoleTenantAdmin, true},
		{RoleOperator, true},
		{RoleReviewer, true},
		{RoleDataSteward, true},
		{RoleWorker, true},
		{Role("root"), false},
		{Role(""), false},
	}
	for _, role := range roles {
		if got := role.value.Valid(); got != role.valid {
			t.Errorf("role %q validity = %v, want %v", role.value, got, role.valid)
		}
	}
	kinds := []struct {
		value StreamKind
		valid bool
	}{
		{StreamPose, true},
		{StreamForce, true},
		{StreamTrajectory, true},
		{StreamFirstPersonVideo, true},
		{StreamThirdPersonVideo, true},
		{StreamKind("audio"), false},
	}
	for _, kind := range kinds {
		if got := kind.value.Valid(); got != kind.valid {
			t.Errorf("stream kind %q validity = %v, want %v", kind.value, got, kind.valid)
		}
	}
}

func TestRigSupportsAllRequiredCapabilities(t *testing.T) {
	t.Parallel()
	rig := CaptureRig{Capabilities: CapabilityPose | CapabilityForce | CapabilityTrajectory | CapabilityFirstPersonVideo}
	if !rig.Supports(CapabilityPose | CapabilityForce) {
		t.Fatal("rig should support required pose and force capabilities")
	}
	if rig.Supports(CapabilityPose | CapabilityOpticalTracking) {
		t.Fatal("rig must reject requirements containing an unsupported capability")
	}
	if rig.Supports(0) {
		t.Fatal("an empty requirement must not be treated as a usable capture capability")
	}
}
