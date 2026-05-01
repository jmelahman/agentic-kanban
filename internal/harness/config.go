package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Source identifies where a resolved harness ID came from. Useful for UI hints.
type Source string

const (
	SourceUser    Source = "user_config"
	SourceProject Source = "project_config"
	SourceDefault Source = "default"
)

type fileConfig struct {
	Harness *struct {
		ID string `toml:"id"`
	} `toml:"harness"`
}

// UserConfigPath returns the path to the user-level kanban config file.
// Honors $XDG_CONFIG_HOME, falling back to $HOME/.config/kanban/config.toml.
func UserConfigPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "kanban", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "kanban", "config.toml"), nil
}

// ReadUserHarness returns the harness ID set in the user config, if any.
func ReadUserHarness() (string, bool) {
	path, err := UserConfigPath()
	if err != nil {
		return "", false
	}
	return readHarnessFile(path)
}

// ReadProjectHarness returns the harness ID set in <repoPath>/.kanban.toml, if any.
func ReadProjectHarness(repoPath string) (string, bool) {
	if repoPath == "" {
		return "", false
	}
	return readHarnessFile(filepath.Join(repoPath, ".kanban.toml"))
}

func readHarnessFile(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var f fileConfig
	if err := toml.Unmarshal(data, &f); err != nil {
		return "", false
	}
	if f.Harness == nil || f.Harness.ID == "" {
		return "", false
	}
	return f.Harness.ID, true
}

// Resolve picks the effective harness for a request, honoring user config,
// then the per-repo project config, then the default. Unknown IDs are skipped
// so a stale entry in either file falls through to the next layer.
func Resolve(repoPath string) (Harness, Source) {
	if id, ok := ReadUserHarness(); ok && IsKnown(id) {
		return Get(id), SourceUser
	}
	if id, ok := ReadProjectHarness(repoPath); ok && IsKnown(id) {
		return Get(id), SourceProject
	}
	return Default(), SourceDefault
}

// WriteUserHarness sets (or clears, when id == "") the user-config harness key,
// preserving any other top-level keys already in the file. Creates the file
// (and parent directories) when needed; deletes the file when clearing leaves
// it empty.
func WriteUserHarness(id string) error {
	if id != "" && !IsKnown(id) {
		return fmt.Errorf("unknown harness %q", id)
	}
	path, err := UserConfigPath()
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
