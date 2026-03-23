package ratelimit

import (
	"fmt"
	"sync"
	"time"
)

const (
	maxFailedAttempts = 5
	windowDuration    = 15 * time.Minute
	cleanupInterval   = 5 * time.Minute
)

type attemptRecord struct {
	count     int
	firstFail time.Time
}

var (
	attempts sync.Map
)

func init() {
	go cleanupLoop()
}

// RecordFailedAttempt increments the failed login counter for the given identifier.
func RecordFailedAttempt(identifier string) {
	now := time.Now()
	val, loaded := attempts.Load(identifier)
	if !loaded {
		attempts.Store(identifier, &attemptRecord{count: 1, firstFail: now})
		return
	}

	record := val.(*attemptRecord)
	// If the window has expired, start a new window
	if now.Sub(record.firstFail) > windowDuration {
		attempts.Store(identifier, &attemptRecord{count: 1, firstFail: now})
		return
	}

	record.count++
}

// IsRateLimited checks whether the identifier is currently locked out.
// Returns true and the remaining lockout duration if rate-limited.
func IsRateLimited(identifier string) (bool, time.Duration) {
	val, ok := attempts.Load(identifier)
	if !ok {
		return false, 0
	}

	record := val.(*attemptRecord)
	elapsed := time.Since(record.firstFail)

	// Window expired — not rate-limited
	if elapsed > windowDuration {
		attempts.Delete(identifier)
		return false, 0
	}

	if record.count >= maxFailedAttempts {
		remaining := windowDuration - elapsed
		return true, remaining
	}

	return false, 0
}

// ResetAttempts clears the failed attempt record for the identifier (e.g. on successful login).
func ResetAttempts(identifier string) {
	attempts.Delete(identifier)
}

// FormatLockoutMessage returns a user-facing message with the remaining lockout time.
func FormatLockoutMessage(remaining time.Duration) string {
	minutes := int(remaining.Minutes()) + 1 // round up
	return fmt.Sprintf("Too many login attempts. Try again in %d minutes.", minutes)
}

// cleanupLoop periodically removes expired entries from the map.
func cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		attempts.Range(func(key, value any) bool {
			record := value.(*attemptRecord)
			if now.Sub(record.firstFail) > windowDuration {
				attempts.Delete(key)
			}
			return true
		})
	}
}
