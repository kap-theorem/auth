package ratelimit

import (
	"testing"
	"time"

	authv1 "authservice/proto/auth/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeClock lets tests advance time deterministically.
type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time               { return f.t }
func (f *fakeClock) advance(d time.Duration)      { f.t = f.t.Add(d) }
func newFakeClock() *fakeClock                    { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }
func withClock(l *Limiter, c *fakeClock) *Limiter { l.now = c.now; return l }

func TestLimiter_LockoutAfterThreshold(t *testing.T) {
	clock := newFakeClock()
	l := withClock(NewLimiter(5, 15*time.Minute), clock)

	for i := 0; i < 5; i++ {
		if !l.Allow("key") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if l.Allow("key") {
		t.Fatalf("6th attempt should be rejected")
	}

	// Other keys are unaffected
	if !l.Allow("other-key") {
		t.Fatalf("different key should be unaffected")
	}
}

func TestLimiter_WindowReset(t *testing.T) {
	clock := newFakeClock()
	l := withClock(NewLimiter(5, 15*time.Minute), clock)

	for i := 0; i < 5; i++ {
		l.Allow("key")
	}
	if l.Allow("key") {
		t.Fatalf("expected lockout after threshold")
	}

	// After a partial window one token refills (15min/5 = 3min per token)
	clock.advance(3 * time.Minute)
	if !l.Allow("key") {
		t.Fatalf("expected one token to refill after 3 minutes")
	}
	if l.Allow("key") {
		t.Fatalf("expected only one token to have refilled")
	}

	// After a full window the bucket is full again
	clock.advance(15 * time.Minute)
	for i := 0; i < 5; i++ {
		if !l.Allow("key") {
			t.Fatalf("attempt %d should be allowed after full window reset", i+1)
		}
	}
}

func TestRateLimiter_LoginPerEmailPerClient(t *testing.T) {
	r := NewRateLimiter()
	method := "/auth.v1.AuthService/GetToken"
	req := func(clientID, email string) *authv1.GetTokenRequest {
		return &authv1.GetTokenRequest{ClientId: clientID, Email: email}
	}

	for i := 0; i < 5; i++ {
		if err := r.Check(method, req("client-a", "alice@example.com")); err != nil {
			t.Fatalf("login attempt %d should pass: %v", i+1, err)
		}
	}
	err := r.Check(method, req("client-a", "alice@example.com"))
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected ResourceExhausted on 6th login attempt, got %v", err)
	}

	// Different email or different client is not affected
	if err := r.Check(method, req("client-a", "bob@example.com")); err != nil {
		t.Fatalf("different email should not be rate limited: %v", err)
	}
	if err := r.Check(method, req("client-b", "alice@example.com")); err != nil {
		t.Fatalf("different client should not be rate limited: %v", err)
	}
}

func TestRateLimiter_RegistrationPerClient(t *testing.T) {
	r := NewRateLimiter()
	method := "/auth.v1.AuthService/RegisterUser"

	for i := 0; i < 10; i++ {
		if err := r.Check(method, &authv1.RegisterUserRequest{ClientId: "client-a"}); err != nil {
			t.Fatalf("registration %d should pass: %v", i+1, err)
		}
	}
	err := r.Check(method, &authv1.RegisterUserRequest{ClientId: "client-a"})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected ResourceExhausted on 11th registration, got %v", err)
	}
	if err := r.Check(method, &authv1.RegisterUserRequest{ClientId: "client-b"}); err != nil {
		t.Fatalf("different client should not be rate limited: %v", err)
	}
}

func TestRateLimiter_GlobalCeiling(t *testing.T) {
	r := NewRateLimiter()
	method := "/auth.v1.AuthService/ValidateToken"

	for i := 0; i < 1000; i++ {
		if err := r.Check(method, &authv1.ValidateTokenRequest{ClientId: "client-a"}); err != nil {
			t.Fatalf("request %d should pass: %v", i+1, err)
		}
	}
	err := r.Check(method, &authv1.ValidateTokenRequest{ClientId: "client-a"})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected ResourceExhausted after per-client ceiling, got %v", err)
	}
}
