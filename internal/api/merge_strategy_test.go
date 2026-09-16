package api_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeKanbanToml drops a .kanban.toml into the env's repo so the per-board
// merge/sync config resolves from it.
func writeKanbanToml(t *testing.T, e *testEnv, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.repoPath, ".kanban.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func errorBody(t *testing.T, e *testEnv, path string, body any, wantCode int) string {
	t.Helper()
	resp := e.post(path, body)
	assertStatus(t, resp, wantCode)
	return decodeJSON[map[string]string](t, resp)["error"]
}

// A strategy the board disables must not be advertised by the validation
// error: naming it costs the caller a round trip to discover it's off.
func TestMergeStrategy_ErrorNamesOnlyEnabledStrategies(t *testing.T) {
	redirectUserConfig(t)
	e := newEnv(t)
	writeKanbanToml(t, e, "[merge]\nallow_merge_commit = false\nallow_rebase = false\n")
	tk := e.seedTicket(e.seedBoard("Merge Cfg"), "T")
	path := fmt.Sprintf("/api/tickets/%d/merge", tk.ID)

	got := errorBody(t, e, path, map[string]any{"strategy": "rebase"}, 400)
	if !strings.Contains(got, "disabled for this board") || !strings.Contains(got, "enabled: squash") {
		t.Fatalf("disabled strategy error = %q, want it to name the enabled set", got)
	}

	// "merge" is a *sync* strategy; the merge equivalent is "merge-commit".
	// The error must list what this board takes, not the static set.
	got = errorBody(t, e, path, map[string]any{"strategy": "merge"}, 400)
	if got != "strategy must be squash" {
		t.Fatalf("unknown strategy error = %q, want %q", got, "strategy must be squash")
	}

	// An enabled strategy clears validation and fails later, on the missing
	// session — proof the gate above rejected on config, not on spelling.
	got = errorBody(t, e, path, map[string]any{"strategy": "squash"}, 404)
	if got != "no session for ticket" {
		t.Fatalf("enabled strategy error = %q, want it to reach the session lookup", got)
	}
}

// The default strategy is subject to the same config, so an omitted strategy
// reports the default as disabled and points at what's left.
func TestSyncStrategy_DisabledDefaultNamesAlternative(t *testing.T) {
	redirectUserConfig(t)
	e := newEnv(t)
	writeKanbanToml(t, e, "[sync]\nallow_rebase = false\n")
	tk := e.seedTicket(e.seedBoard("Sync Cfg"), "T")

	got := errorBody(t, e, fmt.Sprintf("/api/tickets/%d/sync", tk.ID), map[string]any{}, 400)
	if got != "strategy rebase is disabled for this board; enabled: merge" {
		t.Fatalf("omitted strategy error = %q", got)
	}
}

func TestMergeStrategy_AllDisabled(t *testing.T) {
	redirectUserConfig(t)
	e := newEnv(t)
	writeKanbanToml(t, e, "[merge]\nallow_merge_commit = false\nallow_squash = false\nallow_rebase = false\n")
	tk := e.seedTicket(e.seedBoard("No Merge"), "T")

	got := errorBody(t, e, fmt.Sprintf("/api/tickets/%d/merge", tk.ID), map[string]any{"strategy": "squash"}, 400)
	if got != "every strategy is disabled for this board" {
		t.Fatalf("all-disabled error = %q", got)
	}
}
