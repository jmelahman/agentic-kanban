// Package previews bridges kanban and the embedded local-preview
// orchestrator: naming boards for the preview subdomain and executing
// preview build steps inside the target repo's devcontainer.
package previews

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmelahman/local-preview/orchestrator"

	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
)

// BuildsEnv selects preview build execution: "devcontainer" (default when
// Docker is available) or "host".
const BuildsEnv = "KANBAN_PREVIEW_BUILDS"

// AutoDeployEnv disables deploy-on-idle when set to "0" or "false".
const AutoDeployEnv = "KANBAN_PREVIEW_AUTO_DEPLOY"

// RepoName derives the orchestrator repo name — a DNS label, since it
// becomes the subdomain segment — from the board slug.
func RepoName(b *db.Board) string {
	var sb strings.Builder
	for _, c := range strings.ToLower(b.Slug) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			sb.WriteRune(c)
		}
	}
	name := strings.Trim(sb.String(), "-")
	if name == "" {
		return fmt.Sprintf("board-%d", b.ID)
	}
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}
	return name
}

// AutoDeployEnabled reports whether deploy-on-idle is on (the default).
func AutoDeployEnabled() bool {
	v := os.Getenv(AutoDeployEnv)
	return v != "0" && !strings.EqualFold(v, "false")
}

// WorktreeOnboarded reports whether a checkout carries a preview manifest —
// a preview.toml, or a [previews] table in .kanban.toml. It's the cheap
// gate for auto-deploys (the substring probe is a heuristic; the real parse
// happens at build time, where a malformed manifest fails visibly).
func WorktreeOnboarded(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "preview.toml")); err == nil {
		return true
	}
	if b, err := os.ReadFile(filepath.Join(dir, ".kanban.toml")); err == nil {
		return strings.Contains(string(b), "[previews")
	}
	return false
}

// DockerRunner executes preview build steps inside the target repo's
// devcontainer. The config is read from the extracted commit tree — old
// commits build with the environment they shipped with — and resolved
// through kanban's content-addressed image cache, so unchanged configs
// reuse the image. Repos without a devcontainer build in the builtin
// session image.
type DockerRunner struct {
	docker *docker.Client
}

// NewDockerRunner returns a Runner for preview build steps.
func NewDockerRunner(d *docker.Client) *DockerRunner {
	return &DockerRunner{docker: d}
}

// buildUser picks the container user so files written into the bind-mounted
// scratch dir stay owned by the kanban host user: under a rootless daemon
// that's container root (which maps to the host user; the image's default
// non-root user would map to a subordinate uid and can't write the mount),
// under a rootful one it's the host uid:gid itself.
func (r *DockerRunner) buildUser(ctx context.Context) string {
	if r.docker.Rootless(ctx) {
		return "0:0"
	}
	return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
}

// Run implements orchestrator.Runner.
func (r *DockerRunner) Run(ctx context.Context, spec orchestrator.RunSpec, out io.Writer) error {
	cfg, err := docker.LoadDevcontainer(spec.ScratchDir)
	if err != nil {
		return fmt.Errorf("load devcontainer config: %w", err)
	}
	image, err := r.docker.EnsureBuildImage(ctx, cfg, spec.ScratchDir, "preview-"+spec.RepoName)
	if err != nil {
		return fmt.Errorf("resolve devcontainer image: %w", err)
	}
	fmt.Fprintf(out, "[devcontainer image: %s]\n", image)
	if err := r.docker.RunBuildStep(ctx, docker.BuildStep{
		Image:   image,
		HostDir: spec.ScratchDir,
		Dir:     spec.Dir,
		Argv:    spec.Argv,
		Env:     []string{"HOME=/tmp"},
		User:    r.buildUser(ctx),
	}, out); err != nil {
		return fmt.Errorf("devcontainer build: %w", err)
	}
	return nil
}
