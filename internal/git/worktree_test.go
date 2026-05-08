package git

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMergeSquash_IdentityInjected reproduces the "Author identity unknown"
// failure that surfaced when kanban's container has no user.name/user.email
// configured: a clean git repo with no committer config should still be able
// to squash-merge as long as an Identity is passed.
func TestMergeSquash_IdentityInjected(t *testing.T) {
	// Isolate from the developer's global gitconfig so "no identity" really
	// means none — otherwise CI passes but workstations with ~/.gitconfig
	// silently skip the negative branch.
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"EMAIL", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		if _, ok := os.LookupEnv(k); ok {
			old := os.Getenv(k)
			os.Unsetenv(k)
			t.Cleanup(func() { os.Setenv(k, old) })
		}
	}

	repo := initBareishRepo(t)

	// Set identity *only* for the seed commit, then unset it so the merge has
	// to rely on the Identity argument.
	mustGit(t, repo, "config", "user.name", "Seed")
	mustGit(t, repo, "config", "user.email", "seed@example.com")
	writeAndCommit(t, repo, "main", "seed.txt", "seed", "init")
	mustGit(t, repo, "checkout", "-q", "-b", "feature")
	writeAndCommit(t, repo, "feature", "feature.txt", "x", "feature work")
	mustGit(t, repo, "checkout", "-q", "main")
	mustGit(t, repo, "config", "--unset", "user.name")
	mustGit(t, repo, "config", "--unset", "user.email")

	// Sanity: confirm an unconfigured commit fails the way the bug report shows.
	if err := MergeSquash(repo, "feature", "should fail", Identity{}); err == nil {
		t.Fatalf("expected MergeSquash without identity to fail in unconfigured repo")
	}
	// `merge --squash` leaves the index dirty without MERGE_HEAD, so reset
	// rather than `merge --abort`.
	mustGit(t, repo, "reset", "-q", "--hard", "HEAD")

	// With an identity, the same call should succeed.
	id := Identity{Name: "Ada Lovelace", Email: "ada@example.com"}
	if err := MergeSquash(repo, "feature", "squash: feature work", id); err != nil {
		t.Fatalf("MergeSquash with identity: %v", err)
	}

	out := mustGitOut(t, repo, "log", "-1", "--pretty=format:%an <%ae>")
	if got := strings.TrimSpace(out); got != "Ada Lovelace <ada@example.com>" {
		t.Errorf("commit author = %q; want %q", got, "Ada Lovelace <ada@example.com>")
	}
}

func TestIdentity_ConfigArgs(t *testing.T) {
	if got := (Identity{}).configArgs(); got != nil {
		t.Errorf("empty identity should produce no args, got %v", got)
	}
	if got := (Identity{Name: "x"}).configArgs(); got != nil {
		t.Errorf("partial identity should produce no args, got %v", got)
	}
	got := Identity{Name: "Ada", Email: "ada@example.com"}.configArgs()
	want := []string{"-c", "user.name=Ada", "-c", "user.email=ada@example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("arg %d: got %q; want %q", i, got[i], want[i])
		}
	}
}

// --- test helpers ---

func initBareishRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	mustGit(t, "", "init", "-q", "-b", "main", dir)
	return dir
}

func writeAndCommit(t *testing.T, repo, branch, name, contents, msg string) {
	t.Helper()
	cur, _ := CurrentBranch(repo)
	if cur != branch {
		mustGit(t, repo, "checkout", "-q", branch)
	}
	path := filepath.Join(repo, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repo, "add", name)
	mustGit(t, repo, "commit", "-q", "-m", msg)
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, errb.String())
	}
}

func mustGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, errb.String())
	}
	return out.String()
}
