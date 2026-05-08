package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Identity is an optional commit author/committer override. When both fields
// are non-empty, callers can pass it to commit-creating helpers to inject
// `-c user.name=... -c user.email=...` flags so the commit doesn't depend on
// whatever identity the kanban container's git can auto-detect.
type Identity struct {
	Name  string
	Email string
}

func (i Identity) configArgs() []string {
	if i.Name == "" || i.Email == "" {
		return nil
	}
	return []string{"-c", "user.name=" + i.Name, "-c", "user.email=" + i.Email}
}

// AddWorktree creates a new worktree at path, creating a new branch from base.
func AddWorktree(repoPath, branch, worktreePath, base string) error {
	args := []string{"-C", repoPath, "worktree", "add", "-b", branch, worktreePath}
	if base != "" {
		args = append(args, base)
	}
	return run("git", args...)
}

// AddWorktreeFromExisting checks out an existing branch into a new worktree.
func AddWorktreeFromExisting(repoPath, branch, worktreePath string) error {
	return run("git", "-C", repoPath, "worktree", "add", worktreePath, branch)
}

// RemoveWorktree removes a worktree (force).
func RemoveWorktree(repoPath, worktreePath string) error {
	return run("git", "-C", repoPath, "worktree", "remove", "--force", worktreePath)
}

// DeleteBranch deletes a local branch.
func DeleteBranch(repoPath, branch string) error {
	return run("git", "-C", repoPath, "branch", "-D", branch)
}

// Rebase rebases the current branch in worktreePath onto ref.
func Rebase(worktreePath, ref string) error {
	return run("git", "-C", worktreePath, "rebase", ref)
}

// Merge merges ref into the current branch in worktreePath (no edit).
func Merge(worktreePath, ref string) error {
	return run("git", "-C", worktreePath, "merge", "--no-edit", ref)
}

// MergeNoFF merges branch into the currently checked-out branch in repoPath
// always creating a merge commit, even when fast-forward is possible. When
// id is set, its name/email override the commit author/committer.
func MergeNoFF(repoPath, branch string, id Identity) error {
	args := append([]string{}, id.configArgs()...)
	args = append(args, "-C", repoPath, "merge", "--no-ff", "--no-edit", branch)
	return run("git", args...)
}

// MergeSquash squashes branch into the index of repoPath without committing,
// then creates a single commit with message. When id is set, its name/email
// override the commit author/committer.
func MergeSquash(repoPath, branch, message string, id Identity) error {
	if err := run("git", "-C", repoPath, "merge", "--squash", branch); err != nil {
		return err
	}
	args := append([]string{}, id.configArgs()...)
	args = append(args, "-C", repoPath, "commit", "-m", message)
	return run("git", args...)
}

// MergeFFOnly fast-forwards the currently checked-out branch in repoPath to
// branch. Fails if a fast-forward isn't possible.
func MergeFFOnly(repoPath, branch string) error {
	return run("git", "-C", repoPath, "merge", "--ff-only", branch)
}

// CurrentBranch returns the short branch name checked out in repoPath, or an
// empty string if HEAD is detached.
func CurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "symbolic-ref", "--quiet", "--short", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		// Non-zero exit on detached HEAD; surface as empty string, not error.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// RebaseAbort aborts an in-progress rebase. Errors are swallowed.
func RebaseAbort(worktreePath string) {
	_ = exec.Command("git", "-C", worktreePath, "rebase", "--abort").Run()
}

// MergeAbort aborts an in-progress merge. Errors are swallowed.
func MergeAbort(worktreePath string) {
	_ = exec.Command("git", "-C", worktreePath, "merge", "--abort").Run()
}

// ResetHard resets the currently checked-out branch in repoPath to ref.
// Errors are swallowed; used to recover from a failed squash commit.
func ResetHard(repoPath, ref string) {
	_ = exec.Command("git", "-C", repoPath, "reset", "--hard", ref).Run()
}

// AddAll stages every change in the worktree.
func AddAll(worktreePath string) error {
	return run("git", "-C", worktreePath, "add", "-A")
}

// Commit creates a commit with the given message in the worktree. When id is
// set, its name/email override the commit author/committer.
func Commit(worktreePath, message string, id Identity) error {
	args := append([]string{}, id.configArgs()...)
	args = append(args, "-C", worktreePath, "commit", "-m", message)
	return run("git", args...)
}

// IsClean reports whether the worktree has no uncommitted changes.
func IsClean(worktreePath string) (bool, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false, err
	}
	return strings.TrimSpace(out.String()) == "", nil
}

// CurrentHead returns the abbreviated SHA at HEAD.
func CurrentHead(repoPath, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--short", ref)
	var out bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse: %w: %s", err, errb.String())
	}
	return strings.TrimSpace(out.String()), nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, errb.String())
	}
	return nil
}
