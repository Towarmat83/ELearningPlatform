package content

import (
	"sync"
	"time"
)

// ── Cooldown Tracker (in-memory) ──────────────────────────────────────────────────

type CooldownEntry struct {
	Attempts      int
	CooldownUntil time.Time
}

type CooldownTracker struct {
	mu      sync.Mutex
	entries map[string]*CooldownEntry
	ttl     time.Duration
}

func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{
		entries: make(map[string]*CooldownEntry),
		ttl:     30 * time.Minute,
	}
}

func key(userID, contextID, questionID string) string {
	return userID + ":" + contextID + ":" + questionID
}

func keyWithModuleIndex(userID, courseSlug string, moduleIndex int, questionID string) string {
	return userID + ":" + courseSlug + ":" + intToString(moduleIndex) + ":" + questionID
}

func intToString(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var b [20]byte
	bp := len(b) - 1
	for i > 0 {
		b[bp] = byte('0' + i%10)
		i /= 10
		bp--
	}
	if neg {
		b[bp] = '-'
		bp--
	}
	return string(b[bp+1:])
}

func (ct *CooldownTracker) Check(userID, quizID, questionID string) (remaining time.Duration, attempts int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	e, ok := ct.entries[key(userID, quizID, questionID)]
	if !ok {
		return 0, 0
	}
	remaining = time.Until(e.CooldownUntil)
	if remaining < 0 {
		remaining = 0
	}
	return remaining, e.Attempts
}

// CheckModule uses the new module-index-based key format: userID:courseSlug:moduleIndex:questionID.
func (ct *CooldownTracker) CheckModule(userID, courseSlug string, moduleIndex int, questionID string) (remaining time.Duration, attempts int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	k := keyWithModuleIndex(userID, courseSlug, moduleIndex, questionID)
	e, ok := ct.entries[k]
	if !ok {
		return 0, 0
	}
	remaining = time.Until(e.CooldownUntil)
	if remaining < 0 {
		remaining = 0
	}
	return remaining, e.Attempts
}

func (ct *CooldownTracker) Record(userID, quizID, questionID string, spec CooldownSpec, maxAttempts *int, lockOnMax bool) (remaining time.Duration, locked bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	k := key(userID, quizID, questionID)
	e, ok := ct.entries[k]
	if !ok {
		e = &CooldownEntry{}
		ct.entries[k] = e
	}
	e.Attempts++

	if maxAttempts != nil && e.Attempts >= *maxAttempts {
		if lockOnMax {
			return 0, true
		}
	}

	secs := computeCooldown(e.Attempts, spec)
	e.CooldownUntil = time.Now().Add(secs)
	return secs, false
}

// RecordModule uses the new module-index-based key format.
func (ct *CooldownTracker) RecordModule(userID, courseSlug string, moduleIndex int, questionID string, spec CooldownSpec, maxAttempts *int, lockOnMax bool) (remaining time.Duration, locked bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	k := keyWithModuleIndex(userID, courseSlug, moduleIndex, questionID)
	e, ok := ct.entries[k]
	if !ok {
		e = &CooldownEntry{}
		ct.entries[k] = e
	}
	e.Attempts++

	if maxAttempts != nil && e.Attempts >= *maxAttempts {
		if lockOnMax {
			return 0, true
		}
	}

	secs := computeCooldown(e.Attempts, spec)
	e.CooldownUntil = time.Now().Add(secs)
	return secs, false
}

func computeCooldown(currentAttempts int, spec CooldownSpec) time.Duration {
	var secs int
	switch spec.Strategy {
	case "fixed":
		secs = spec.BaseSeconds
	case "linear":
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

func (ct *CooldownTracker) Clear(userID, quizID, questionID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	delete(ct.entries, key(userID, quizID, questionID))
}

// ClearModule uses the new module-index-based key format.
func (ct *CooldownTracker) ClearModule(userID, courseSlug string, moduleIndex int, questionID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	delete(ct.entries, keyWithModuleIndex(userID, courseSlug, moduleIndex, questionID))
}
