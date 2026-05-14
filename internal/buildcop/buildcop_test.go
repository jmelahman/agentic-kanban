package buildcop

import (
	"strings"
	"testing"
	"time"

	"github.com/jmelahman/kanban/internal/kanbantoml"
)

func toBoardSection(name, repoPath, branch string, threshold *float64, minRuns, streak, window *int) kanbantoml.BuildCopBoard {
	out := kanbantoml.BuildCopBoard{}
	if name != "" {
		out.Name = &name
	}
	if repoPath != "" {
		out.RepoPath = &repoPath
	}
	if branch != "" {
		out.Branch = &branch
	}
	out.FailureThreshold = threshold
	out.MinRuns = minRuns
	out.GreenStreakRequired = streak
	out.WindowDays = window
	return out
}

func TestComputeFingerprintStability(t *testing.T) {
	a := computeFingerprint("CI", "lint")
	b := computeFingerprint("CI", "lint")
	if a != b {
		t.Errorf("same inputs produced different fingerprints: %s vs %s", a, b)
	}
	if len(a) != 16 {
		t.Errorf("fingerprint length = %d, want 16", len(a))
	}
}

func TestComputeFingerprintDistinct(t *testing.T) {
	cases := [][2]string{
		{"CI", "lint"},
		{"CI", "test"},
		{"Build", "lint"},
		{"CI:Lint", ""}, // pathological: ':' in workflow
		{"CI", ":Lint"},
	}
	seen := map[string]string{}
	for _, c := range cases {
		fp := computeFingerprint(c[0], c[1])
		key := c[0] + "|" + c[1]
		if dup, ok := seen[fp]; ok && dup != key {
			t.Errorf("collision: %s and %s both -> %s", dup, key, fp)
		}
		seen[fp] = key
	}
}

func TestClassifyConclusion(t *testing.T) {
	cases := map[string]string{
		"success":         "success",
		"SUCCESS":         "success",
		"neutral":         "success",
		"failure":         "failure",
		"timed_out":       "failure",
		"action_required": "failure",
		"cancelled":       "skip",
		"skipped":         "skip",
		"":                "skip",
		"unknown":         "skip",
	}
	for in, want := range cases {
		if got := classifyConclusion(in); got != want {
			t.Errorf("classifyConclusion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShouldFile(t *testing.T) {
	cfg := BoardConfig{FailureThreshold: 0.10, MinRuns: 5}
	cases := []struct {
		name string
		s    jobStats
		want bool
	}{
		{name: "below min_runs even with all failures", s: jobStats{Total: 4, Failures: 4}, want: false},
		{name: "min_runs but under threshold", s: jobStats{Total: 10, Failures: 1}, want: false},        // 10% (not strictly greater)
		{name: "min_runs and above threshold", s: jobStats{Total: 10, Failures: 2}, want: true},         // 20%
		{name: "at min_runs boundary, above threshold", s: jobStats{Total: 5, Failures: 1}, want: true}, // 20%
		{name: "zero total", s: jobStats{Total: 0, Failures: 0}, want: false},
	}
	for _, c := range cases {
		if got := shouldFile(c.s, cfg); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestShouldFix(t *testing.T) {
	cfg := BoardConfig{GreenStreakRequired: 10}
	cases := []struct {
		name string
		s    jobStats
		want bool
	}{
		{name: "no streak", s: jobStats{GreenStreak: 0}, want: false},
		{name: "short streak", s: jobStats{GreenStreak: 9}, want: false},
		{name: "exact streak", s: jobStats{GreenStreak: 10}, want: true},
		{name: "long streak", s: jobStats{GreenStreak: 50}, want: true},
	}
	for _, c := range cases {
		if got := shouldFix(c.s, cfg); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// runAt returns a ghRun at minute m of an arbitrary base time. Newer m =
// newer run. Tests sort entirely by CreatedAt, so the exact base date is
// irrelevant.
func runAt(id int64, workflow string, m int) ghRun {
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	return ghRun{
		ID:        id,
		Name:      workflow,
		CreatedAt: base.Add(time.Duration(m) * time.Minute),
		HTMLURL:   "https://example.com/runs/x",
	}
}

func jobAt(name, conclusion, htmlURL string) ghJob {
	return ghJob{Name: name, Conclusion: conclusion, HTMLURL: htmlURL}
}

func TestAggregateGreenStreak(t *testing.T) {
	// Five runs of "CI" newest→oldest with the "lint" job:
	//   run5 success, run4 success, run3 failure, run2 success, run1 success.
	// GreenStreak should be 2 (leading two successes only), total 5,
	// failures 1, successes 4.
	runs := []ghRun{
		runAt(1, "CI", 1),
		runAt(2, "CI", 2),
		runAt(3, "CI", 3),
		runAt(4, "CI", 4),
		runAt(5, "CI", 5),
	}
	jobsByRun := map[int64][]ghJob{
		1: {jobAt("lint", "success", "")},
		2: {jobAt("lint", "success", "")},
		3: {jobAt("lint", "failure", "https://example.com/jobs/3")},
		4: {jobAt("lint", "success", "")},
		5: {jobAt("lint", "success", "")},
	}
	stats := aggregate(runs, jobsByRun)
	s, ok := stats["CI:lint"]
	if !ok {
		t.Fatalf("missing CI:lint, got %v", stats)
	}
	if s.Total != 5 || s.Successes != 4 || s.Failures != 1 {
		t.Errorf("counts: total=%d, successes=%d, failures=%d", s.Total, s.Successes, s.Failures)
	}
	if s.GreenStreak != 2 {
		t.Errorf("GreenStreak = %d, want 2", s.GreenStreak)
	}
	if s.LastFailureURL != "https://example.com/jobs/3" {
		t.Errorf("LastFailureURL = %q", s.LastFailureURL)
	}
}

func TestAggregateSplitsByWorkflowAndJob(t *testing.T) {
	runs := []ghRun{runAt(1, "CI", 1), runAt(2, "Build", 2)}
	jobsByRun := map[int64][]ghJob{
		1: {jobAt("lint", "success", ""), jobAt("test", "failure", "")},
		2: {jobAt("lint", "failure", "")},
	}
	stats := aggregate(runs, jobsByRun)
	if len(stats) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(stats), keysOf(stats))
	}
	for _, key := range []string{"CI:lint", "CI:test", "Build:lint"} {
		if _, ok := stats[key]; !ok {
			t.Errorf("missing %q in %v", key, keysOf(stats))
		}
	}
}

func TestAggregateSkipsCancelled(t *testing.T) {
	runs := []ghRun{runAt(1, "CI", 1), runAt(2, "CI", 2)}
	jobsByRun := map[int64][]ghJob{
		1: {jobAt("lint", "cancelled", "")},
		2: {jobAt("lint", "success", "")},
	}
	stats := aggregate(runs, jobsByRun)
	s := stats["CI:lint"]
	if s.Total != 1 {
		t.Errorf("Total = %d, want 1 (cancelled skipped)", s.Total)
	}
	if s.GreenStreak != 1 {
		t.Errorf("GreenStreak = %d, want 1", s.GreenStreak)
	}
}

func TestAggregateAllFailures(t *testing.T) {
	runs := []ghRun{runAt(1, "CI", 1), runAt(2, "CI", 2)}
	jobsByRun := map[int64][]ghJob{
		1: {jobAt("lint", "failure", "https://example.com/old")},
		2: {jobAt("lint", "failure", "https://example.com/new")},
	}
	stats := aggregate(runs, jobsByRun)
	s := stats["CI:lint"]
	if s.GreenStreak != 0 {
		t.Errorf("GreenStreak = %d, want 0", s.GreenStreak)
	}
	if s.LastFailureURL != "https://example.com/new" {
		t.Errorf("LastFailureURL = %q, want newest run's URL", s.LastFailureURL)
	}
}

func TestTitleFor(t *testing.T) {
	s := jobStats{Workflow: "CI", Job: "lint", Total: 20, Failures: 3}
	got := titleFor(s)
	if !strings.Contains(got, "CI / lint") {
		t.Errorf("title missing workflow/job: %q", got)
	}
	if !strings.Contains(got, "15%") {
		t.Errorf("title missing rate: %q", got)
	}
	if !strings.Contains(got, "20 runs") {
		t.Errorf("title missing count: %q", got)
	}
}

func TestResolveBoardDefaults(t *testing.T) {
	got := resolveBoard(toBoardSection("", "", "", nil, nil, nil, nil))
	if got.FailureThreshold != 0.10 {
		t.Errorf("FailureThreshold default = %v", got.FailureThreshold)
	}
	if got.MinRuns != 5 {
		t.Errorf("MinRuns default = %d", got.MinRuns)
	}
	if got.GreenStreakRequired != 10 {
		t.Errorf("GreenStreakRequired default = %d", got.GreenStreakRequired)
	}
	if got.WindowDays != 7 {
		t.Errorf("WindowDays default = %d", got.WindowDays)
	}
	if !got.MatchesAllBranches() {
		t.Errorf("empty branch should match all")
	}
}

func TestResolveBoardNameFallback(t *testing.T) {
	master := "master"
	got := resolveBoard(toBoardSection("", "", master, nil, nil, nil, nil))
	if !strings.Contains(got.Name, "master") {
		t.Errorf("name should derive from branch: %q", got.Name)
	}
	if got.Slug == "" {
		t.Errorf("slug should be derived from name")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Build Cop: master": "build-cop-master",
		"  Hello   World  ": "hello-world",
		"":                  "build-cop",
		"@@@":               "build-cop",
		"Build Cop / all":   "build-cop-all",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func keysOf(m map[string]jobStats) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
