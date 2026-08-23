package integration_test

import (
	"context"
	"sync"
	"testing"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func TestTask0013SealIncludesCommittedFinalSegment(t *testing.T) {
	e := newEnvironment(t)
	fixture := e.validatedCapture(t)
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE capture_sessions SET status = 'recording', version = version + 1 WHERE id = ?`, fixture.plan.Capture.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.database.SQL().ExecContext(context.Background(), `UPDATE stream_manifests SET status = 'open', digest = '', version = version + 1 WHERE id = ?`, fixture.pose.ID); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, err := e.streams.Seal(context.Background(), e.operator, fixture.pose.ID, "task0013-a")
		errs <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		_, err := e.streams.Seal(context.Background(), e.operator, fixture.pose.ID, "task0013-b")
		errs <- err
	}()
	close(start)
	wait.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent seal committed %d terminal transitions", successes)
	}
	manifest, err := e.streams.List(context.Background(), e.operator, fixture.plan.Capture.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) == 0 || manifest[0].Status != domain.ManifestSealed {
		t.Fatalf("manifest did not end sealed: %+v", manifest)
	}
}
