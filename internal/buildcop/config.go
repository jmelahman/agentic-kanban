package buildcop

import (
	"fmt"

	"github.com/jmelahman/kanban/internal/kanbantoml"
)

// Config is the resolved [buildcop] section: a list of board configurations
// to poll. Enabled is the master switch; even when true, an empty Boards
// slice means the poller is a no-op.
type Config struct {
	Enabled bool
	Boards  []BoardConfig
}

// BoardConfig is one Build Cop board's full set of knobs after defaults
// have been applied. RepoPath is the only field with no useful default.
type BoardConfig struct {
	Name                string
	Slug                string
	RepoPath            string
	Branch              string  // "" or "*" means all branches
	FailureThreshold    float64 // 0..1, default 0.10
	MinRuns             int     // default 5
	GreenStreakRequired int     // default 10
	WindowDays          int     // default 7
}

// MatchesAllBranches reports whether this board accepts every head_branch.
func (b BoardConfig) MatchesAllBranches() bool {
	return b.Branch == "" || b.Branch == "*"
}

// ResolveConfig reads .kanban.toml (project + user, merged) and returns a
// Config with defaults applied per board. Disabled when the [buildcop]
// section is absent or `enabled = false`.
func ResolveConfig(repoPath string) Config {
	cfg := Config{Enabled: false}
	f := kanbantoml.Load(repoPath)
	if f.BuildCop == nil {
		return cfg
	}
	if f.BuildCop.Enabled != nil {
		cfg.Enabled = *f.BuildCop.Enabled
	}
	for _, b := range f.BuildCop.Boards {
		cfg.Boards = append(cfg.Boards, resolveBoard(b))
	}
	return cfg
}

func resolveBoard(in kanbantoml.BuildCopBoard) BoardConfig {
	out := BoardConfig{
		FailureThreshold:    0.10,
		MinRuns:             5,
		GreenStreakRequired: 10,
		WindowDays:          7,
	}
	if in.RepoPath != nil {
		out.RepoPath = *in.RepoPath
	}
	if in.Branch != nil {
		out.Branch = *in.Branch
	}
	if in.Name != nil && *in.Name != "" {
		out.Name = *in.Name
	} else {
		out.Name = defaultName(out.Branch)
	}
	out.Slug = slugify(out.Name)
	if in.FailureThreshold != nil {
		out.FailureThreshold = *in.FailureThreshold
	}
	if in.MinRuns != nil && *in.MinRuns > 0 {
		out.MinRuns = *in.MinRuns
	}
	if in.GreenStreakRequired != nil && *in.GreenStreakRequired > 0 {
		out.GreenStreakRequired = *in.GreenStreakRequired
	}
	if in.WindowDays != nil && *in.WindowDays > 0 {
		out.WindowDays = *in.WindowDays
	}
	return out
}

func defaultName(branch string) string {
	if branch == "" || branch == "*" {
		return "Build Cop: all branches"
	}
	return fmt.Sprintf("Build Cop: %s", branch)
}
