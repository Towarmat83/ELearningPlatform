package content

import (
	"testing"
	"time"
)

func TestIntToString(t *testing.T) {
	cases := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-1, "-1"},
		{-100, "-100"},
		{9999, "9999"},
	}
	for _, tc := range cases {
		got := intToString(tc.input)
		if got != tc.want {
			t.Errorf("intToString(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestKey(t *testing.T) {
	k := key("user1", "quiz1", "q1")

	expected := "user1:quiz1:q1"
	if k != expected {
		t.Errorf("expected %q, got %q", expected, k)
	}
}

func TestKeyWithModuleIndex(t *testing.T) {
	k := keyWithModuleIndex("user1", "course1", 3, "q1")

	expected := "user1:course1:3:q1"
	if k != expected {
		t.Errorf("expected %q, got %q", expected, k)
	}
}

func TestNewCooldownTracker(t *testing.T) {
	ct := NewCooldownTracker()
	if ct == nil {
		t.Fatal("expected non-nil CooldownTracker")
	}

	if ct.entries == nil {
		t.Error("expected non-nil entries map")
	}

	if ct.ttl != 30*time.Minute {
		t.Errorf("expected 30m TTL, got %v", ct.ttl)
	}
}

func TestComputeCooldown_Fixed(t *testing.T) {
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: 30}

	d := computeCooldown(1, spec)
	if d != 30*time.Second {
		t.Errorf("expected 30s, got %v", d)
	}

	d2 := computeCooldown(5, spec)
	if d2 != 30*time.Second {
		t.Errorf("fixed strategy should return constant 30s, got %v", d2)
	}
}

func TestComputeCooldown_Linear(t *testing.T) {
	spec := CooldownSpec{Strategy: "linear", BaseSeconds: 10}

	d1 := computeCooldown(1, spec)
	if d1 != 10*time.Second {
		t.Errorf("expected 10s for attempt 1, got %v", d1)
	}

	d3 := computeCooldown(3, spec)
	if d3 != 30*time.Second {
		t.Errorf("expected 30s for attempt 3, got %v", d3)
	}
}

func TestComputeCooldown_Exponential(t *testing.T) {
	spec := CooldownSpec{Strategy: "exponential", BaseSeconds: 10, Multiplier: 2.0}

	d1 := computeCooldown(1, spec)
	if d1 != 10*time.Second {
		t.Errorf("expected 10s for attempt 1, got %v", d1)
	}

	d2 := computeCooldown(2, spec)
	if d2 != 20*time.Second {
		t.Errorf("expected 20s for attempt 2, got %v", d2)
	}

	d3 := computeCooldown(3, spec)
	if d3 != 40*time.Second {
		t.Errorf("expected 40s for attempt 3, got %v", d3)
	}
}

func TestComputeCooldown_ExponentialMax(t *testing.T) {
	spec := CooldownSpec{Strategy: "exponential", BaseSeconds: 10, Multiplier: 2.0, MaxSeconds: 25}

	d3 := computeCooldown(3, spec)
	if d3 != 25*time.Second {
		t.Errorf("expected capped at 25s, got %v", d3)
	}
}

func TestComputeCooldown_DefaultExponential(t *testing.T) {
	// Empty strategy defaults to exponential
	spec := CooldownSpec{BaseSeconds: 5, Multiplier: 3.0}

	d := computeCooldown(1, spec)
	if d != 5*time.Second {
		t.Errorf("expected 5s, got %v", d)
	}
}

func TestCooldownTracker_Check_Initial(t *testing.T) {
	ct := NewCooldownTracker()

	remaining, attempts := ct.Check("u", "q", "q1")
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %v", remaining)
	}

	if attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", attempts)
	}
}

func TestCooldownTracker_Record_Fixed(t *testing.T) {
	ct := NewCooldownTracker()
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: 60}

	remaining, locked := ct.Record("u1", "quiz1", "q1", spec, nil, false)
	if locked {
		t.Error("expected not locked")
	}

	if remaining <= 0 {
		t.Errorf("expected positive remaining, got %v", remaining)
	}

	remaining2, attempts := ct.Check("u1", "quiz1", "q1")
	if remaining2 <= 0 {
		t.Errorf("expected positive remaining after record, got %v", remaining2)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestCooldownTracker_Record_MaxAttempts_Lock(t *testing.T) {
	ct := NewCooldownTracker()
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: 5}
	max := 2

	ct.Record("u1", "quiz1", "q1", spec, &max, true)

	_, locked := ct.Record("u1", "quiz1", "q1", spec, &max, true)
	if !locked {
		t.Error("expected locked after max attempts")
	}
}

func TestCooldownTracker_Record_MaxAttempts_NoLock(t *testing.T) {
	ct := NewCooldownTracker()
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: 5}
	max := 1

	ct.Record("u1", "quiz1", "q1", spec, &max, false)

	_, locked := ct.Record("u1", "quiz1", "q1", spec, &max, false)
	if locked {
		t.Error("expected not locked when lockOnMax=false")
	}
}

func TestCooldownTracker_Clear(t *testing.T) {
	ct := NewCooldownTracker()
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: 60}
	ct.Record("u1", "quiz1", "q1", spec, nil, false)

	ct.Clear("u1", "quiz1", "q1")

	remaining, attempts := ct.Check("u1", "quiz1", "q1")
	if remaining != 0 || attempts != 0 {
		t.Errorf("expected cleared entry, got remaining=%v attempts=%d", remaining, attempts)
	}
}

func TestCooldownTracker_ClearModule(t *testing.T) {
	ct := NewCooldownTracker()
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: 60}
	ct.RecordModule("u1", "course1", 2, "q1", spec, nil, false)

	ct.ClearModule("u1", "course1", 2, "q1")

	remaining, attempts := ct.CheckModule("u1", "course1", 2, "q1")
	if remaining != 0 || attempts != 0 {
		t.Errorf("expected cleared, got remaining=%v attempts=%d", remaining, attempts)
	}
}

func TestCooldownTracker_CheckModule_Initial(t *testing.T) {
	ct := NewCooldownTracker()

	remaining, attempts := ct.CheckModule("u", "course", 0, "q")
	if remaining != 0 || attempts != 0 {
		t.Error("expected zero for unknown module entry")
	}
}

func TestCooldownTracker_RecordModule(t *testing.T) {
	ct := NewCooldownTracker()
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: 30}

	remaining, locked := ct.RecordModule("u1", "course1", 1, "q1", spec, nil, false)
	if locked {
		t.Error("expected not locked")
	}

	if remaining <= 0 {
		t.Errorf("expected positive remaining, got %v", remaining)
	}

	_, attempts := ct.CheckModule("u1", "course1", 1, "q1")
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestCooldownTracker_Expired(t *testing.T) {
	ct := NewCooldownTracker()
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: 0}
	ct.Record("u1", "quiz1", "q1", spec, nil, false)

	// With 0 seconds cooldown, it should be expired immediately
	remaining, _ := ct.Check("u1", "quiz1", "q1")
	if remaining < 0 {
		t.Errorf("remaining should be 0 (not negative), got %v", remaining)
	}
}

func TestCooldownTracker_CheckModule_Expired(t *testing.T) {
	ct := NewCooldownTracker()
	// Negative BaseSeconds sets CooldownUntil in the past → remaining < 0 → clamped to 0.
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: -10}
	ct.RecordModule("u1", "course1", 0, "q1", spec, nil, false)

	remaining, attempts := ct.CheckModule("u1", "course1", 0, "q1")
	if remaining != 0 {
		t.Errorf("expected 0 remaining for past cooldown, got %v", remaining)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestIntToString_Negative(t *testing.T) {
	if got := intToString(-5); got != "-5" {
		t.Errorf("intToString(-5): want -5, got %q", got)
	}
}

func TestIntToString_Zero(t *testing.T) {
	if got := intToString(0); got != "0" {
		t.Errorf("intToString(0): want 0, got %q", got)
	}
}

func TestIntToString_Positive(t *testing.T) {
	if got := intToString(42); got != "42" {
		t.Errorf("intToString(42): want 42, got %q", got)
	}
}

func TestCooldownTracker_RecordModule_MaxAttempts_Lock(t *testing.T) {
	ct := NewCooldownTracker()
	spec := CooldownSpec{Strategy: "fixed", BaseSeconds: 30}
	maxAttempts := 1
	// First attempt hits maxAttempts with lockOnMax=true → should return locked=true
	_, locked := ct.RecordModule("u1", "course1", 0, "q1", spec, &maxAttempts, true)
	if !locked {
		t.Error("expected locked=true when maxAttempts reached with lockOnMax")
	}
}
