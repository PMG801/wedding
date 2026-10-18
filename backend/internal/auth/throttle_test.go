package auth

import (
	"context"
	"testing"
	"time"
)

// fakeClock advances only when the throttle sleeps, so no test waits in real time.
type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	return nil
}

func TestLoginThrottleDoublesDelayAndCapsItAfterConsecutiveFailures(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	throttle := NewLoginThrottle(ThrottleOptions{Now: clock.Now, Sleep: clock.Sleep})

	if err := throttle.Wait(context.Background()); err != nil || len(clock.sleeps) != 0 {
		t.Fatalf("first attempt slept %v; want no delay", clock.sleeps)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	for i, delay := range want {
		throttle.RecordFailure()
		if err := throttle.Wait(context.Background()); err != nil {
			t.Fatalf("Wait() after failure %d = %v", i+1, err)
		}
		if got := clock.sleeps[len(clock.sleeps)-1]; got != delay {
			t.Fatalf("delay after failure %d = %s; want %s", i+1, got, delay)
		}
	}
}

func TestLoginThrottleResetsOnSuccess(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	throttle := NewLoginThrottle(ThrottleOptions{Now: clock.Now, Sleep: clock.Sleep})
	throttle.RecordFailure()
	throttle.RecordFailure()
	throttle.RecordFailure()
	throttle.RecordSuccess()

	if err := throttle.Wait(context.Background()); err != nil || len(clock.sleeps) != 0 {
		t.Fatalf("attempt after success slept %v; want no delay", clock.sleeps)
	}
	throttle.RecordFailure()
	if err := throttle.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := clock.sleeps[len(clock.sleeps)-1]; got != time.Second {
		t.Fatalf("first failure after success delayed %s; want 1s", got)
	}
}

func TestLoginThrottleHonoursElapsedTime(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	throttle := NewLoginThrottle(ThrottleOptions{Now: clock.Now, Sleep: clock.Sleep})
	throttle.RecordFailure()
	clock.now = clock.now.Add(5 * time.Second)
	if err := throttle.Wait(context.Background()); err != nil || len(clock.sleeps) != 0 {
		t.Fatalf("Wait() after the delay elapsed slept %v; want none", clock.sleeps)
	}
}
