package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A result that arrives before its task's transaction commits is stored once
// the task shows up.
func TestWaitForTaskUntilItShowsUp(t *testing.T) {
	calls := 0
	err := waitForTask(context.Background(), 5, time.Millisecond, func() error {
		calls++
		if calls < 3 {
			return errUnknownTask
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Errorf("err %v after %d calls, want nil after 3", err, calls)
	}
}

func TestWaitForTaskGivesUp(t *testing.T) {
	calls := 0
	err := waitForTask(context.Background(), 4, time.Millisecond, func() error {
		calls++
		return errUnknownTask
	})
	if !errors.Is(err, errUnknownTask) || calls != 4 {
		t.Errorf("err %v after %d calls, want errUnknownTask after 4", err, calls)
	}
}

func TestWaitForTaskDoesNotRetryOtherErrors(t *testing.T) {
	calls := 0
	boom := errors.New("database down")
	err := waitForTask(context.Background(), 4, time.Millisecond, func() error {
		calls++
		return boom
	})
	if !errors.Is(err, boom) || calls != 1 {
		t.Errorf("err %v after %d calls, want the error at once", err, calls)
	}
}
