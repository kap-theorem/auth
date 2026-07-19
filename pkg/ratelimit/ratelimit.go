// Package ratelimit provides an in-memory token-bucket rate limiter and a
// gRPC unary interceptor enforcing the Phase 1 limits:
//   - Login:        5 attempts / email / client / 15 minutes
//   - Registration: 10 / client / hour
//   - Per-client request ceiling: 1000 / minute
//
// Buckets live in process memory; if the service is ever replicated the
// backing store must move to Redis (see design spec §5).
package ratelimit

import (
	"context"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter is a keyed token bucket: each key gets `capacity` tokens that
// refill continuously at capacity/window.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64
	window   time.Duration
	now      func() time.Time // injectable clock for tests
}

func NewLimiter(capacity int, window time.Duration) *Limiter {
	return &Limiter{
		buckets:  make(map[string]*bucket),
		capacity: float64(capacity),
		window:   window,
		now:      time.Now,
	}
}

// Allow consumes one token for key, returning false when the bucket is empty.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	} else {
		refill := now.Sub(b.last).Seconds() * (l.capacity / l.window.Seconds())
		b.tokens += refill
		if b.tokens > l.capacity {
			b.tokens = l.capacity
		}
		b.last = now
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// RateLimiter bundles the three Phase 1 limits.
type RateLimiter struct {
	login    *Limiter // keyed by client_id|email
	register *Limiter // keyed by client_id
	global   *Limiter // keyed by client_id
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		login:    NewLimiter(5, 15*time.Minute),
		register: NewLimiter(10, time.Hour),
		global:   NewLimiter(1000, time.Minute),
	}
}

// clientIDer / emailer are satisfied by the generated request types.
type clientIDer interface{ GetClientId() string }
type emailer interface{ GetEmail() string }

// Check applies the limits for a request; a non-nil error means rejected.
func (r *RateLimiter) Check(fullMethod string, req interface{}) error {
	clientID := ""
	if c, ok := req.(clientIDer); ok {
		clientID = c.GetClientId()
	}
	if clientID == "" {
		return nil // client authentication will reject the request anyway
	}

	// Any request carrying a client_id — including the hosted account RPCs
	// (HostedGetProfile / HostedChangePassword / HostedRevokeSession /
	// HostedLogoutAll) — is subject to the per-client global ceiling. Those
	// methods need no per-email login limiter: they never take a password
	// guess, only an already-issued access token.
	if !r.global.Allow(clientID) {
		return status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}

	switch {
	// HostedLogin is a client-secret-free login path; it shares GetToken's
	// per-email/per-client login limiter (same buckets).
	case strings.HasSuffix(fullMethod, "/GetToken"), strings.HasSuffix(fullMethod, "/HostedLogin"):
		email := ""
		if e, ok := req.(emailer); ok {
			email = e.GetEmail()
		}
		if !r.login.Allow(clientID + "|" + email) {
			return status.Error(codes.ResourceExhausted, "too many login attempts, try again later")
		}
	case strings.HasSuffix(fullMethod, "/RegisterUser"):
		if !r.register.Allow(clientID) {
			return status.Error(codes.ResourceExhausted, "too many registrations, try again later")
		}
	}
	return nil
}

// UnaryInterceptor returns a gRPC unary server interceptor enforcing the limits.
func (r *RateLimiter) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := r.Check(info.FullMethod, req); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}
