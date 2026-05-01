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
	Harness *HarnessSection `toml:"harness"`
	Sync    *SyncSection    `toml:"sync"`
	Merge   *MergeSection   `toml:"merge"`
	GitHub  *GitHubSection  `toml:"github"`
	Tasks   []TaskEntry     `toml:"task"`
}

type HarnessSection struct {
	ID *string `toml:"id"`
}

type SyncSection struct {
	AllowRebase *bool `toml:"allow_rebase"`
	AllowMerge  *bool `toml:"allow_merge"`
}

type MergeSection struct {
	AllowMergeCommit *bool `toml:"allow_merge_commit"`
	AllowSquash      *bool `toml:"allow_squash"`
	AllowRebase      *bool `toml:"allow_rebase"`
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

// UserPath returns the user-level config path.
func UserPath() (string, error) {
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
	out.Tasks = mergeTasks(project.Tasks, user.Tasks)

	return out
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

	if id == "" {
		delete(root, "harness")
	} else {
		root["harness"] = map[string]any{"id": id}
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
