package auth

import (
	"context"
	"sync"
	"time"
)

const (
	defaultThrottleBase = time.Second
	defaultThrottleMax  = 30 * time.Second
)

// ThrottleOptions injects the clock and sleeper; zero values use real time.
type ThrottleOptions struct {
	Now   func() time.Time
	Sleep func(context.Context, time.Duration) error
	Base  time.Duration
	Max   time.Duration
}

// LoginThrottle delays administrator logins after consecutive failures. It is
// global and holds only a counter and a deadline, so its memory is constant.
// Each failure doubles the delay applied before the next attempt, up to Max;
// a success resets it.
type LoginThrottle struct {
	attempt      sync.Mutex // one wait-verify-record cycle at a time
	mu           sync.Mutex
	now          func() time.Time
	sleep        func(context.Context, time.Duration) error
	base, max    time.Duration
	failures     int
	blockedUntil time.Time
}

// NewLoginThrottle builds a throttle with a 1s base delay capped at 30s unless overridden.
func NewLoginThrottle(options ThrottleOptions) *LoginThrottle {
	throttle := &LoginThrottle{now: options.Now, sleep: options.Sleep, base: options.Base, max: options.Max}
	if throttle.now == nil {
		throttle.now = time.Now
	}
	if throttle.sleep == nil {
		throttle.sleep = sleepContext
	}
	if throttle.base <= 0 {
		throttle.base = defaultThrottleBase
	}
	if throttle.max <= 0 {
		throttle.max = defaultThrottleMax
	}
	return throttle
}

// Attempt runs one login attempt: it waits out any delay left by earlier
// failures, verifies, and records the outcome, holding a lock for the whole
// cycle. Serializing makes the delay apply to every guess, including
// concurrent ones, and bounds password hashing (argon2id or bcrypt) to one
// verification at a time on a small host.
func (t *LoginThrottle) Attempt(ctx context.Context, verify func() bool) (bool, error) {
	t.attempt.Lock()
	defer t.attempt.Unlock()
	if err := t.Wait(ctx); err != nil {
		return false, err
	}
	if !verify() {
		t.RecordFailure()
		return false, nil
	}
	t.RecordSuccess()
	return true, nil
}

// Wait blocks until any delay left by earlier failures has passed.
func (t *LoginThrottle) Wait(ctx context.Context) error {
	t.mu.Lock()
	remaining := t.blockedUntil.Sub(t.now())
	t.mu.Unlock()
	if remaining <= 0 {
		return nil
	}
	return t.sleep(ctx, remaining)
}

// RecordFailure extends the delay applied before the next attempt.
func (t *LoginThrottle) RecordFailure() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failures < 64 {
		t.failures++
	}
	delay := t.base
	for i := 1; i < t.failures && delay < t.max; i++ {
		delay *= 2
	}
	if delay > t.max {
		delay = t.max
	}
	t.blockedUntil = t.now().Add(delay)
}

// RecordSuccess clears the failure history.
func (t *LoginThrottle) RecordSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures = 0
	t.blockedUntil = time.Time{}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
