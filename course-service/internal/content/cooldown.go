// Package content implements course content loading, quiz scoring, and
// related domain logic for the course-service.
package content

import (
	"sync"
	"time"
)

// cooldownDefaultTTL is the default time-to-live for cached cooldown
// entries.
const cooldownDefaultTTL = 30 * time.Minute

// Cooldown strategy identifiers understood by computeCooldown.
const (
	cooldownStrategyFixed       = "fixed"
	cooldownStrategyLinear      = "linear"
	cooldownStrategyExponential = "exponential"
)

// ── Cooldown Tracker (in-memory) ──────────────────────────────────

// CooldownEntry tracks the attempt count and cooldown expiry for a single
// question.
type CooldownEntry struct {
	Attempts      int
	CooldownUntil time.Time
}

// CooldownTracker stores per-question cooldown state in memory.
type CooldownTracker struct {
	mu      sync.Mutex
	entries map[string]*CooldownEntry
	ttl     time.Duration
}

// NewCooldownTracker creates an empty CooldownTracker using the default
// TTL.
func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{
		entries: make(map[string]*CooldownEntry),
		ttl:     cooldownDefaultTTL,
	}
}

// key builds the legacy quiz-based cache key.
func key(userID, contextID, questionID string) string {
	return userID + ":" + contextID + ":" + questionID
}

// keyWithModuleIndex builds the module-index-based cache key.
func keyWithModuleIndex(userID, courseSlug string, moduleIndex int, questionID string) string {
	return userID + ":" + courseSlug + ":" + intToString(moduleIndex) + ":" + questionID
}

// intToString converts an int to its base-10 string form without pulling
// in strconv, keeping this package's dependency footprint minimal.
func intToString(num int) string {
	if num == 0 {
		return "0"
	}

	neg := false
	if num < 0 {
		neg = true
		num = -num
	}

	var digits [20]byte

	idx := len(digits) - 1
	for num > 0 {
		digits[idx] = byte('0' + num%10)
		num /= 10
		idx--
	}

	if neg {
		digits[idx] = '-'
		idx--
	}

	return string(digits[idx+1:])
}

// Check reports the remaining cooldown and attempt count for the legacy
// quiz-based key format.
func (ct *CooldownTracker) Check(userID, quizID, questionID string) (remaining time.Duration, attempts int) { //nolint:nonamedreturns // gocritic(unnamedResult) wants names here
	ct.mu.Lock()
	defer ct.mu.Unlock()

	entry, ok := ct.entries[key(userID, quizID, questionID)]
	if !ok {
		return 0, 0
	}

	remaining = max(time.Until(entry.CooldownUntil), 0)

	return remaining, entry.Attempts
}

// CheckModule uses the module-index-based key format:
// userID:courseSlug:moduleIndex:questionID.
func (ct *CooldownTracker) CheckModule(userID, courseSlug string, moduleIndex int, questionID string) (remaining time.Duration, attempts int) { //nolint:nonamedreturns // gocritic(unnamedResult) wants names here
	ct.mu.Lock()
	defer ct.mu.Unlock()

	k := keyWithModuleIndex(userID, courseSlug, moduleIndex, questionID)

	entry, ok := ct.entries[k]
	if !ok {
		return 0, 0
	}

	remaining = max(time.Until(entry.CooldownUntil), 0)

	return remaining, entry.Attempts
}

// Record increments the attempt count for the legacy quiz-based key format
// and returns the resulting cooldown duration and lock state.
func (ct *CooldownTracker) Record(userID, quizID, questionID string, spec CooldownSpec, maxAttempts *int, lockOnMax bool) (time.Duration, bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	k := key(userID, quizID, questionID)

	entry, ok := ct.entries[k]
	if !ok {
		entry = &CooldownEntry{}
		ct.entries[k] = entry
	}

	entry.Attempts++

	if maxAttempts != nil && entry.Attempts >= *maxAttempts {
		if lockOnMax {
			return 0, true
		}
	}

	secs := computeCooldown(entry.Attempts, spec)
	entry.CooldownUntil = time.Now().Add(secs)

	return secs, false
}

// RecordModule uses the module-index-based key format.
func (ct *CooldownTracker) RecordModule(userID, courseSlug string, moduleIndex int, questionID string, spec CooldownSpec, maxAttempts *int, lockOnMax bool) (time.Duration, bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	k := keyWithModuleIndex(userID, courseSlug, moduleIndex, questionID)

	entry, ok := ct.entries[k]
	if !ok {
		entry = &CooldownEntry{}
		ct.entries[k] = entry
	}

	entry.Attempts++

	if maxAttempts != nil && entry.Attempts >= *maxAttempts {
		if lockOnMax {
			return 0, true
		}
	}

	secs := computeCooldown(entry.Attempts, spec)
	entry.CooldownUntil = time.Now().Add(secs)

	return secs, false
}

// computeCooldown calculates the cooldown duration for the given attempt
// count according to the strategy configured in spec.
func computeCooldown(currentAttempts int, spec CooldownSpec) time.Duration {
	var secs int

	switch spec.Strategy {
	case cooldownStrategyFixed:
		secs = spec.BaseSeconds
	case cooldownStrategyLinear:
		secs = spec.BaseSeconds * currentAttempts
	default: // exponential
		mult := 1.0
		for i := 1; i < currentAttempts; i++ {
			mult *= spec.Multiplier
		}

		secs = int(float64(spec.BaseSeconds) * mult)
		if spec.MaxSeconds > 0 && secs > spec.MaxSeconds {
			secs = spec.MaxSeconds
		}
	}

	return time.Duration(secs) * time.Second
}

// Clear removes the cooldown entry for the legacy quiz-based key format.
func (ct *CooldownTracker) Clear(userID, quizID, questionID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	delete(ct.entries, key(userID, quizID, questionID))
}

// ClearModule uses the module-index-based key format.
func (ct *CooldownTracker) ClearModule(userID, courseSlug string, moduleIndex int, questionID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	delete(ct.entries, keyWithModuleIndex(userID, courseSlug, moduleIndex, questionID))
}
