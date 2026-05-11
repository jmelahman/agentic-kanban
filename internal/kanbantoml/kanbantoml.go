// Package kanbantoml reads `.kanban.toml` from two layers — the user config
// at $XDG_CONFIG_HOME/kanban/config.toml (falling back to
// $HOME/.config/kanban/config.toml) and the per-repo file at
// <repoPath>/.kanban.toml — and merges them with user-wins semantics.
//
// All scalar fields use *T so an absent key is distinguishable from a zero
// value: the user file only overrides keys it actually sets. Tasks merge by
// label — a user entry with the same label replaces the project entry;
// user-only labels are appended.
package kanbantoml

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type File struct {
	Harness      *HarnessSection      `toml:"harness"`
	Sync         *SyncSection         `toml:"sync"`
	Merge        *MergeSection        `toml:"merge"`
	GitHub       *GitHubSection       `toml:"github"`
	Worktrees    *WorktreesSection    `toml:"worktrees"`
	Branches     *BranchesSection     `toml:"branches"`
	Devcontainer *DevcontainerSection `toml:"devcontainer"`
	Errors       *ErrorsSection       `toml:"errors"`
	Agent        *AgentSection        `toml:"agent"`
	Tasks        []TaskEntry          `toml:"task"`
}

type HarnessSection struct {
	ID *string `toml:"id"`
}

type WorktreesSection struct {
	Root *string `toml:"root"`
}

// BranchesSection sets defaults for the branch name new sessions get. Prefix
// is a literal string concatenated with "/" + ticket.Slug to form the full
// branch name. Per-board overrides in the boards table take precedence.
type BranchesSection struct {
	Prefix *string `toml:"prefix"`
}

// AgentSection toggles agent auto-start on session boot. When AutoStart is
// true and the ticket body is non-empty, kanban writes the body to
// .kanban/prompt.txt inside the worktree and execs the harness's
// StartCommandTemplate inside the container after spawn. Default false:
// sessions boot to an idle container and the agent only runs once a user
// attaches to the PTY.
type AgentSection struct {
	AutoStart *bool `toml:"auto_start"`
}

// ErrorsSection toggles the in-process error-to-ticket reporter. Disabled by
// default — only developers maintaining the app typically want application
// errors mixed into their kanban boards.
type ErrorsSection struct {
	Enabled   *bool   `toml:"enabled"`
	BoardName *string `toml:"board_name"`
}

type SyncSection struct {
	AllowRebase *bool `toml:"allow_rebase"`
	AllowMerge  *bool `toml:"allow_merge"`
}

type MergeSection struct {
	AllowMergeCommit *bool `toml:"allow_merge_commit"`
	AllowSquash      *bool `toml:"allow_squash"`
	AllowRebase      *bool `toml:"allow_rebase"`
	// AICommitMessage opts in to harness-generated commit messages for the
	// auto-commit kanban makes when a session has uncommitted changes at merge
	// time. Default false: kanban uses the ticket title.
	AICommitMessage *bool `toml:"ai_commit_message"`
}

// DevcontainerSection augments the loaded devcontainer.json. Mounts and
// run_args append to whatever the devcontainer.json already declares;
// container_env merges key-by-key with user values winning over both the
// project file and the devcontainer.json.
//
// DockerSocket and ClaudeConfig toggle host bind mounts on the *built-in*
// devcontainer only — hand-written devcontainer.json files are unaffected
// and manage their own mounts. Both default to true when unset.
type DevcontainerSection struct {
	RunArgs      []string          `toml:"run_args"`
	Mounts       []string          `toml:"mounts"`
	ContainerEnv map[string]string `toml:"container_env"`
	DockerSocket *bool             `toml:"docker_socket"`
	ClaudeConfig *bool             `toml:"claude_config"`
}

type GitHubSection struct {
	AutoMove     *bool   `toml:"auto_move"`
	DraftColumn  *string `toml:"draft_column"`
	ReviewColumn *string `toml:"review_column"`
	DoneColumn   *string `toml:"done_column"`
	ClosedColumn *string `toml:"closed_column"`
}

type TaskEntry struct {
	Label         string `toml:"label"`
	ContainerPort int    `toml:"container_port"`
}

// UserPath returns the user-level config path. $KANBAN_CONFIG, when set,
// overrides the default location so callers (e.g. the `serve --config` flag)
// can point at an arbitrary file without threading the path through every
// caller of Load.
func UserPath() (string, error) {
	if path := os.Getenv("KANBAN_CONFIG"); path != "" {
		return path, nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "kanban", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "kanban", "config.toml"), nil
}

// ProjectPath returns the per-repo config path. Empty repoPath returns "".
func ProjectPath(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	return filepath.Join(repoPath, ".kanban.toml")
}

// Load reads the user file and the project file (either may be missing or
// unparseable, in which case it is treated as empty) and returns a merged
// File where user values win.
func Load(repoPath string) File {
	project := readFileAt(ProjectPath(repoPath))
	user := File{}
	if path, err := UserPath(); err == nil {
		user = readFileAt(path)
	}
	return merge(project, user)
}

func readFileAt(path string) File {
	if path == "" {
		return File{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}
	}
	var f File
	if err := toml.Unmarshal(data, &f); err != nil {
		return File{}
	}
	return f
}

// merge combines project (low priority) and user (high priority) into one
// File. Each scalar field falls through unless the user set it; tasks merge
// by label.
func merge(project, user File) File {
	out := File{}

	out.Harness = mergeHarness(project.Harness, user.Harness)
	out.Sync = mergeSync(project.Sync, user.Sync)
	out.Merge = mergeMerge(project.Merge, user.Merge)
	out.GitHub = mergeGitHub(project.GitHub, user.GitHub)
	out.Worktrees = mergeWorktrees(project.Worktrees, user.Worktrees)
	out.Branches = mergeBranches(project.Branches, user.Branches)
	out.Devcontainer = mergeDevcontainer(project.Devcontainer, user.Devcontainer)
	out.Errors = mergeErrors(project.Errors, user.Errors)
	out.Agent = mergeAgent(project.Agent, user.Agent)
	out.Tasks = mergeTasks(project.Tasks, user.Tasks)

	return out
}

func mergeWorktrees(p, u *WorktreesSection) *WorktreesSection {
	if p == nil && u == nil {
		return nil
	}
	out := WorktreesSection{}
	if p != nil {
		out.Root = p.Root
	}
	if u != nil && u.Root != nil {
		out.Root = u.Root
	}
	return &out
}

func mergeBranches(p, u *BranchesSection) *BranchesSection {
	if p == nil && u == nil {
		return nil
	}
	out := BranchesSection{}
	if p != nil {
		out.Prefix = p.Prefix
	}
	if u != nil && u.Prefix != nil {
		out.Prefix = u.Prefix
	}
	return &out
}

func mergeAgent(p, u *AgentSection) *AgentSection {
	if p == nil && u == nil {
		return nil
	}
	out := AgentSection{}
	if p != nil {
		out.AutoStart = p.AutoStart
	}
	if u != nil && u.AutoStart != nil {
		out.AutoStart = u.AutoStart
	}
	return &out
}

func mergeErrors(p, u *ErrorsSection) *ErrorsSection {
	if p == nil && u == nil {
		return nil
	}
	out := ErrorsSection{}
	if p != nil {
		out.Enabled = p.Enabled
		out.BoardName = p.BoardName
	}
	if u != nil {
		if u.Enabled != nil {
			out.Enabled = u.Enabled
		}
		if u.BoardName != nil {
			out.BoardName = u.BoardName
		}
	}
	return &out
}

func mergeHarness(p, u *HarnessSection) *HarnessSection {
	if p == nil && u == nil {
		return nil
	}
	out := HarnessSection{}
	if p != nil {
		out.ID = p.ID
	}
	if u != nil && u.ID != nil {
		out.ID = u.ID
	}
	return &out
}

func mergeSync(p, u *SyncSection) *SyncSection {
	if p == nil && u == nil {
		return nil
	}
	out := SyncSection{}
	if p != nil {
		out.AllowRebase = p.AllowRebase
		out.AllowMerge = p.AllowMerge
	}
	if u != nil {
		if u.AllowRebase != nil {
			out.AllowRebase = u.AllowRebase
		}
		if u.AllowMerge != nil {
			out.AllowMerge = u.AllowMerge
		}
	}
	return &out
}

func mergeMerge(p, u *MergeSection) *MergeSection {
	if p == nil && u == nil {
		return nil
	}
	out := MergeSection{}
	if p != nil {
		out.AllowMergeCommit = p.AllowMergeCommit
		out.AllowSquash = p.AllowSquash
		out.AllowRebase = p.AllowRebase
		out.AICommitMessage = p.AICommitMessage
	}
	if u != nil {
		if u.AllowMergeCommit != nil {
			out.AllowMergeCommit = u.AllowMergeCommit
		}
		if u.AllowSquash != nil {
			out.AllowSquash = u.AllowSquash
		}
		if u.AllowRebase != nil {
			out.AllowRebase = u.AllowRebase
		}
		if u.AICommitMessage != nil {
			out.AICommitMessage = u.AICommitMessage
		}
	}
	return &out
}

func mergeGitHub(p, u *GitHubSection) *GitHubSection {
	if p == nil && u == nil {
		return nil
	}
	out := GitHubSection{}
	if p != nil {
		out = *p
	}
	if u != nil {
		if u.AutoMove != nil {
			out.AutoMove = u.AutoMove
		}
		if u.DraftColumn != nil {
			out.DraftColumn = u.DraftColumn
		}
		if u.ReviewColumn != nil {
			out.ReviewColumn = u.ReviewColumn
		}
		if u.DoneColumn != nil {
			out.DoneColumn = u.DoneColumn
		}
		if u.ClosedColumn != nil {
			out.ClosedColumn = u.ClosedColumn
		}
	}
	return &out
}

func mergeDevcontainer(p, u *DevcontainerSection) *DevcontainerSection {
	if p == nil && u == nil {
		return nil
	}
	out := DevcontainerSection{}
	if p != nil {
		out.RunArgs = append(out.RunArgs, p.RunArgs...)
		out.Mounts = append(out.Mounts, p.Mounts...)
		for k, v := range p.ContainerEnv {
			if out.ContainerEnv == nil {
				out.ContainerEnv = map[string]string{}
			}
			out.ContainerEnv[k] = v
		}
		out.DockerSocket = p.DockerSocket
		out.ClaudeConfig = p.ClaudeConfig
	}
	if u != nil {
		out.RunArgs = append(out.RunArgs, u.RunArgs...)
		out.Mounts = append(out.Mounts, u.Mounts...)
		for k, v := range u.ContainerEnv {
			if out.ContainerEnv == nil {
				out.ContainerEnv = map[string]string{}
			}
			out.ContainerEnv[k] = v
		}
		if u.DockerSocket != nil {
			out.DockerSocket = u.DockerSocket
		}
		if u.ClaudeConfig != nil {
			out.ClaudeConfig = u.ClaudeConfig
		}
	}
	return &out
}

func mergeTasks(project, user []TaskEntry) []TaskEntry {
	if len(project) == 0 && len(user) == 0 {
		return nil
	}
	byLabel := make(map[string]int, len(project))
	out := make([]TaskEntry, 0, len(project)+len(user))
	for _, t := range project {
		byLabel[t.Label] = len(out)
		out = append(out, t)
	}
	for _, t := range user {
		if idx, ok := byLabel[t.Label]; ok {
			out[idx] = t
			continue
		}
		byLabel[t.Label] = len(out)
		out = append(out, t)
	}
	return out
}

// PortFor returns the container_port for the named task, if any.
func (f File) PortFor(label string) (int, bool) {
	for _, t := range f.Tasks {
		if t.Label == label && t.ContainerPort > 0 {
			return t.ContainerPort, true
		}
	}
	return 0, false
}

// WriteUserHarness sets (or clears, when id == "") the [harness].id key in
// the user config, preserving any other top-level keys already in the file.
// Creates the file (and parent directories) when needed; deletes the file
// when clearing leaves it empty.
func WriteUserHarness(id string) error {
	if id == "" {
		return writeUserSection("harness", nil)
	}
	return writeUserSection("harness", map[string]any{"id": id})
}

// WriteUserWorktreesRoot sets (or clears, when root == "") the
// [worktrees].root key in the user config.
func WriteUserWorktreesRoot(root string) error {
	if root == "" {
		return writeUserSection("worktrees", nil)
	}
	return writeUserSection("worktrees", map[string]any{"root": root})
}

// writeUserSection sets the named top-level table to value (or removes it
// when value is nil), preserving every other top-level key already in the
// file. Deletes the file when the result is empty.
func writeUserSection(name string, value map[string]any) error {
	path, err := UserPath()
	if err != nil {
		return err
	}

	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := toml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if value == nil {
		delete(root, name)
	} else {
		root[name] = value
	}

	if len(root) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := toml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}
