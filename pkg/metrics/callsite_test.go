package metrics

import (
	"testing"
)

func TestCallSiteTracker_HitRate(t *testing.T) {
	tr := NewCallSiteTracker()

	// First request: cache write (no reads yet).
	tr.Record("agent.go:42", UsageRecord{
		CacheCreationInputTokens: 8000,
		InputTokens:              200,
	})

	s := tr.Stats("agent.go:42")
	if s == nil {
		t.Fatal("expected stats, got nil")
	}
	if s.HitRate() != 0 {
		t.Errorf("expected 0 hit rate after write-only request, got %f", s.HitRate())
	}
	if s.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", s.TotalRequests)
	}

	// Second request: cache read.
	tr.Record("agent.go:42", UsageRecord{
		CacheReadInputTokens: 8000,
	})

	s = tr.Stats("agent.go:42")
	// hit rate = 8000 / (8000 + 8000 + 200) = ~0.49
	if s.HitRate() <= 0 {
		t.Errorf("expected positive hit rate after cache read, got %f", s.HitRate())
	}
	if s.CacheHitRequests != 1 {
		t.Errorf("expected 1 cache hit request, got %d", s.CacheHitRequests)
	}
	if s.RequestHitRate() != 0.5 {
		t.Errorf("expected 0.5 request hit rate, got %f", s.RequestHitRate())
	}
}

func TestCallSiteTracker_WriteEfficiency(t *testing.T) {
	tr := NewCallSiteTracker()

	tr.Record("planner.go:84", UsageRecord{CacheCreationInputTokens: 4000})
	tr.Record("planner.go:84", UsageRecord{CacheReadInputTokens: 4000})
	tr.Record("planner.go:84", UsageRecord{CacheReadInputTokens: 4000})

	s := tr.Stats("planner.go:84")
	// efficiency = 8000 / 4000 = 2.0
	if s.WriteEfficiency() != 2.0 {
		t.Errorf("expected write efficiency 2.0, got %f", s.WriteEfficiency())
	}
}

func TestCallSiteTracker_AllStats_SortedByHitRate(t *testing.T) {
	tr := NewCallSiteTracker()

	// good: 100% hit rate
	tr.Record("good.go:1", UsageRecord{CacheReadInputTokens: 1000})
	// bad: 0% hit rate
	tr.Record("bad.go:1", UsageRecord{CacheCreationInputTokens: 1000})

	all := tr.AllStats()
	if len(all) != 2 {
		t.Fatalf("expected 2 records, got %d", len(all))
	}
	// Worst first.
	if all[0].CallSite != "bad.go:1" {
		t.Errorf("expected bad.go:1 first (worst hit rate), got %s", all[0].CallSite)
	}
}

func TestCallSiteTracker_Reset(t *testing.T) {
	tr := NewCallSiteTracker()
	tr.Record("x.go:1", UsageRecord{InputTokens: 100})
	tr.Reset("x.go:1")
	if tr.Stats("x.go:1") != nil {
		t.Error("expected nil after reset")
	}
}

func TestCallSiteTracker_ResetAll(t *testing.T) {
	tr := NewCallSiteTracker()
	tr.Record("a.go:1", UsageRecord{InputTokens: 100})
	tr.Record("b.go:1", UsageRecord{InputTokens: 100})
	tr.ResetAll()
	if len(tr.AllStats()) != 0 {
		t.Error("expected empty after ResetAll")
	}
}

func TestCallSiteTracker_Summary(t *testing.T) {
	tr := NewCallSiteTracker()
	tr.Record("agent.go:42", UsageRecord{
		CacheCreationInputTokens: 4000,
		CacheReadInputTokens:     4000,
	})
	s := tr.Summary()
	if s == "" || s == "no call sites recorded" {
		t.Error("expected non-empty summary")
	}
}

func TestCallSiteTracker_NilStats(t *testing.T) {
	tr := NewCallSiteTracker()
	if tr.Stats("nonexistent.go:1") != nil {
		t.Error("expected nil for unknown call site")
	}
}
