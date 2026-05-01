package api

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// MergeConfig captures the per-board merge strategies allowed by the project's
// .kanban.toml. Defaults are all-true so existing repos behave unchanged.
type MergeConfig struct {
	AllowMergeCommit bool `json:"allow_merge_commit"`
	AllowSquash      bool `json:"allow_squash"`
	AllowRebase      bool `json:"allow_rebase"`
}

func defaultMergeConfig() MergeConfig {
	return MergeConfig{AllowMergeCommit: true, AllowSquash: true, AllowRebase: true}
}

// SyncConfig captures the per-board sync strategies allowed by the project's
// .kanban.toml. Defaults are all-true so existing repos behave unchanged.
type SyncConfig struct {
	AllowRebase bool `json:"allow_rebase"`
	AllowMerge  bool `json:"allow_merge"`
}

func defaultSyncConfig() SyncConfig {
	return SyncConfig{AllowRebase: true, AllowMerge: true}
}

type kanbanTOML struct {
	Merge *struct {
		AllowMergeCommit *bool `toml:"allow_merge_commit"`
		AllowSquash      *bool `toml:"allow_squash"`
		AllowRebase      *bool `toml:"allow_rebase"`
	} `toml:"merge"`
	Sync *struct {
		AllowRebase *bool `toml:"allow_rebase"`
		AllowMerge  *bool `toml:"allow_merge"`
	} `toml:"sync"`
}

func readKanbanTOML(repoPath string) (kanbanTOML, bool) {
	if repoPath == "" {
		return kanbanTOML{}, false
	}
	data, err := os.ReadFile(filepath.Join(repoPath, ".kanban.toml"))
	if err != nil {
		return kanbanTOML{}, false
	}
	var f kanbanTOML
	if err := toml.Unmarshal(data, &f); err != nil {
		return kanbanTOML{}, false
	}
	return f, true
}

// loadMergeConfig reads <repoPath>/.kanban.toml and applies any [merge]
// overrides to the defaults. Missing file or unparseable TOML yields defaults.
func loadMergeConfig(repoPath string) MergeConfig {
	cfg := defaultMergeConfig()
	f, ok := readKanbanTOML(repoPath)
	if !ok || f.Merge == nil {
		return cfg
	}
	if f.Merge.AllowMergeCommit != nil {
		cfg.AllowMergeCommit = *f.Merge.AllowMergeCommit
	}
	if f.Merge.AllowSquash != nil {
		cfg.AllowSquash = *f.Merge.AllowSquash
	}
	if f.Merge.AllowRebase != nil {
		cfg.AllowRebase = *f.Merge.AllowRebase
	}
	return cfg
}

// loadSyncConfig reads <repoPath>/.kanban.toml and applies any [sync]
// overrides to the defaults. Missing file or unparseable TOML yields defaults.
func loadSyncConfig(repoPath string) SyncConfig {
	cfg := defaultSyncConfig()
	f, ok := readKanbanTOML(repoPath)
	if !ok || f.Sync == nil {
		return cfg
	}
	if f.Sync.AllowRebase != nil {
		cfg.AllowRebase = *f.Sync.AllowRebase
	}
	if f.Sync.AllowMerge != nil {
		cfg.AllowMerge = *f.Sync.AllowMerge
	}
	return cfg
}

func (mc MergeConfig) allows(strategy string) bool {
	switch strategy {
	case "merge-commit":
		return mc.AllowMergeCommit
	case "squash":
		return mc.AllowSquash
	case "rebase":
		return mc.AllowRebase
	}
	return false
}

func (sc SyncConfig) allows(strategy string) bool {
	switch strategy {
	case "rebase":
		return sc.AllowRebase
	case "merge":
		return sc.AllowMerge
	}
	return false
}
