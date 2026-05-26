package buildcop

import "testing"

func TestJobsCacheHitMiss(t *testing.T) {
	p := &Poller{jobsCache: make(map[string][]ghJob)}

	if _, ok := p.getCachedJobs("repo-a", 1); ok {
		t.Fatal("empty cache should miss")
	}

	jobs := []ghJob{{ID: 10, Conclusion: "failure"}}
	p.putCachedJobs("repo-a", 1, jobs)

	got, ok := p.getCachedJobs("repo-a", 1)
	if !ok {
		t.Fatal("expected hit after put")
	}
	if len(got) != 1 || got[0].ID != 10 {
		t.Errorf("got %+v; want jobs[0].ID=10", got)
	}

	// Different repo with same run_id must not collide.
	if _, ok := p.getCachedJobs("repo-b", 1); ok {
		t.Error("repo-b should miss; key collides with repo-a")
	}
}

func TestJobsCacheRetainEvictsStale(t *testing.T) {
	p := &Poller{jobsCache: make(map[string][]ghJob)}
	p.putCachedJobs("repo-a", 1, nil)
	p.putCachedJobs("repo-a", 2, nil)
	p.putCachedJobs("repo-a", 3, nil)
	p.putCachedJobs("repo-b", 1, nil) // different repo, must survive

	p.retainJobs("repo-a", map[int64]struct{}{1: {}, 3: {}})

	if _, ok := p.getCachedJobs("repo-a", 1); !ok {
		t.Error("repo-a/1 evicted; should be retained")
	}
	if _, ok := p.getCachedJobs("repo-a", 2); ok {
		t.Error("repo-a/2 retained; should have been evicted")
	}
	if _, ok := p.getCachedJobs("repo-a", 3); !ok {
		t.Error("repo-a/3 evicted; should be retained")
	}
	if _, ok := p.getCachedJobs("repo-b", 1); !ok {
		t.Error("repo-b/1 evicted by repo-a's retainJobs; scopes leaked")
	}
}
