package cache

import (
	"testing"
	"time"
)

func TestTTLTracker_FirstTouch_ColdStart(t *testing.T) {
	tr := NewTTLTracker(5 * time.Minute)
	alive := tr.Touch("hash-abc")
	if alive {
		t.Error("expected cold start (wasAlive=false) on first touch")
	}
	e := tr.Entry("hash-abc")
	if e == nil {
		t.Fatal("expected entry after touch")
	}
	if e.MissCount != 1 {
		t.Errorf("expected MissCount=1, got %d", e.MissCount)
	}
}

func TestTTLTracker_SecondTouch_WarmHit(t *testing.T) {
	tr := NewTTLTracker(5 * time.Minute)
	tr.Touch("hash-abc")
	alive := tr.Touch("hash-abc")
	if !alive {
		t.Error("expected warm hit (wasAlive=true) on second touch within TTL")
	}
	e := tr.Entry("hash-abc")
	if e.HitCount != 1 {
		t.Errorf("expected HitCount=1, got %d", e.HitCount)
	}
}

func TestTTLTracker_ExpiredEntry(t *testing.T) {
	// Use a very short TTL to simulate expiry.
	tr := NewTTLTracker(1 * time.Millisecond)
	tr.Touch("hash-xyz")
	time.Sleep(5 * time.Millisecond)

	alive := tr.Touch("hash-xyz")
	if alive {
		t.Error("expected cold start after TTL expiry")
	}
	e := tr.Entry("hash-xyz")
	if !e.Expired {
		t.Error("expected Expired=true after TTL expiry")
	}
}

func TestTTLTracker_TimeUntilExpiry(t *testing.T) {
	tr := NewTTLTracker(5 * time.Minute)
	tr.Touch("hash-abc")
	d := tr.TimeUntilExpiry("hash-abc")
	if d <= 0 || d > 5*time.Minute {
		t.Errorf("expected 0 < expiry <= 5m, got %v", d)
	}
}

func TestTTLTracker_TimeUntilExpiry_Unknown(t *testing.T) {
	tr := NewTTLTracker(5 * time.Minute)
	if tr.TimeUntilExpiry("unknown") != 0 {
		t.Error("expected 0 for unknown prefix")
	}
}

func TestTTLTracker_ScheduleDeadline(t *testing.T) {
	tr := NewTTLTracker(5 * time.Minute)
	tr.Touch("hash-abc")
	margin := 30 * time.Second
	deadline := tr.ScheduleDeadline("hash-abc", margin)
	if deadline.IsZero() {
		t.Fatal("expected non-zero deadline")
	}
	// Deadline should be ~4m30s from now.
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 5*time.Minute {
		t.Errorf("unexpected deadline remaining: %v", remaining)
	}
}

func TestTTLTracker_ExpiredEntries(t *testing.T) {
	tr := NewTTLTracker(1 * time.Millisecond)
	tr.Touch("a")
	tr.Touch("b")
	time.Sleep(5 * time.Millisecond)
	tr.Touch("c") // fresh entry

	expired := tr.ExpiredEntries()
	if len(expired) != 2 {
		t.Errorf("expected 2 expired entries, got %d", len(expired))
	}
}

func TestTTLTracker_Evict(t *testing.T) {
	tr := NewTTLTracker(5 * time.Minute)
	tr.Touch("hash-abc")
	tr.Evict("hash-abc")
	if tr.Entry("hash-abc") != nil {
		t.Error("expected nil after eviction")
	}
}

func TestTTLTracker_Stats(t *testing.T) {
	tr := NewTTLTracker(5 * time.Minute)
	tr.Touch("a")
	tr.Touch("a") // hit
	tr.Touch("b")

	s := tr.Stats()
	if s.TotalPrefixes != 2 {
		t.Errorf("expected 2 prefixes, got %d", s.TotalPrefixes)
	}
	if s.AlivePrefixes != 2 {
		t.Errorf("expected 2 alive, got %d", s.AlivePrefixes)
	}
	if s.TotalHits != 1 {
		t.Errorf("expected 1 total hit, got %d", s.TotalHits)
	}
}
