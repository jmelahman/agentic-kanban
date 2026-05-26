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

func TestResolveLatestBase(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	t.Setenv("HOME", t.TempDir())

	t.Run("origin_strictly_ahead", func(t *testing.T) {
		remote, local := seedRemoteAndClone(t)
		// Push a newer commit to the bare remote via a second clone.
		other := filepath.Join(t.TempDir(), "other")
		mustGit(t, "", "clone", "-q", remote, other)
		mustGit(t, other, "config", "user.name", "Other")
		mustGit(t, other, "config", "user.email", "other@example.com")
		writeAndCommit(t, other, "main", "new.txt", "new", "newer")
		mustGit(t, other, "push", "-q", "origin", "main")

		got := ResolveLatestBase(local, "main")
		if got != "origin/main" {
			t.Fatalf("got %q, want %q", got, "origin/main")
		}
		// Sanity: the fetch actually happened — origin/main now matches the new commit.
		remoteSha := strings.TrimSpace(mustGitOut(t, local, "rev-parse", "refs/remotes/origin/main"))
		otherSha := strings.TrimSpace(mustGitOut(t, other, "rev-parse", "HEAD"))
		if remoteSha != otherSha {
			t.Fatalf("origin/main = %q, want %q (fetch didn't run)", remoteSha, otherSha)
		}
	})

	t.Run("local_strictly_ahead", func(t *testing.T) {
		_, local := seedRemoteAndClone(t)
		writeAndCommit(t, local, "main", "unpushed.txt", "x", "unpushed")
		if got := ResolveLatestBase(local, "main"); got != "main" {
			t.Fatalf("got %q, want %q (must preserve unpushed work)", got, "main")
		}
	})

	t.Run("equal_tips", func(t *testing.T) {
		_, local := seedRemoteAndClone(t)
		if got := ResolveLatestBase(local, "main"); got != "main" {
			t.Fatalf("got %q, want %q", got, "main")
		}
	})

	t.Run("diverged", func(t *testing.T) {
		remote, local := seedRemoteAndClone(t)
		// Local commits one thing on main.
		writeAndCommit(t, local, "main", "local.txt", "L", "local-only")
		// Remote (via second clone) commits a different thing on main and pushes.
		other := filepath.Join(t.TempDir(), "other")
		mustGit(t, "", "clone", "-q", remote, other)
		mustGit(t, other, "config", "user.name", "Other")
		mustGit(t, other, "config", "user.email", "other@example.com")
		writeAndCommit(t, other, "main", "remote.txt", "R", "remote-only")
		mustGit(t, other, "push", "-q", "origin", "main")

		if got := ResolveLatestBase(local, "main"); got != "main" {
			t.Fatalf("got %q, want %q (diverged: prefer local)", got, "main")
		}
	})

	t.Run("no_remote", func(t *testing.T) {
		repo := initBareishRepo(t)
		mustGit(t, repo, "config", "user.name", "Solo")
		mustGit(t, repo, "config", "user.email", "solo@example.com")
		writeAndCommit(t, repo, "main", "seed.txt", "s", "seed")
		if got := ResolveLatestBase(repo, "main"); got != "main" {
			t.Fatalf("got %q, want %q", got, "main")
		}
	})

	t.Run("empty_base", func(t *testing.T) {
		repo := initBareishRepo(t)
		if got := ResolveLatestBase(repo, ""); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

// --- test helpers ---

// seedRemoteAndClone creates a bare "remote" with one commit on main, then
// clones it into a "local" working repo. Returns absolute paths to both.
func seedRemoteAndClone(t *testing.T) (remote, local string) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	mustGit(t, "", "init", "-q", "--bare", "-b", "main", remote)

	// Seed via a temporary clone so the bare repo gets an initial main branch.
	seed := filepath.Join(root, "seed")
	mustGit(t, "", "clone", "-q", remote, seed)
	mustGit(t, seed, "config", "user.name", "Seed")
	mustGit(t, seed, "config", "user.email", "seed@example.com")
	mustGit(t, seed, "checkout", "-q", "-b", "main")
	writeAndCommit(t, seed, "main", "seed.txt", "seed", "seed")
	mustGit(t, seed, "push", "-q", "-u", "origin", "main")

	local = filepath.Join(root, "local")
	mustGit(t, "", "clone", "-q", remote, local)
	mustGit(t, local, "config", "user.name", "Local")
	mustGit(t, local, "config", "user.email", "local@example.com")
	return remote, local
}

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
