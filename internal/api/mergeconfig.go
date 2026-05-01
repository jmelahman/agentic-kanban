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

type mergeTOML struct {
	Merge *struct {
		AllowMergeCommit *bool `toml:"allow_merge_commit"`
		AllowSquash      *bool `toml:"allow_squash"`
		AllowRebase      *bool `toml:"allow_rebase"`
	} `toml:"merge"`
}

// loadMergeConfig reads <repoPath>/.kanban.toml and applies any [merge]
// overrides to the defaults. Missing file or unparseable TOML yields defaults.
func loadMergeConfig(repoPath string) MergeConfig {
	cfg := defaultMergeConfig()
	if repoPath == "" {
		return cfg
	}
	data, err := os.ReadFile(filepath.Join(repoPath, ".kanban.toml"))
	if err != nil {
		return cfg
	}
	var f mergeTOML
	if err := toml.Unmarshal(data, &f); err != nil {
		return cfg
	}
	if f.Merge == nil {
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
