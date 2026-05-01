package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
)

// DevcontainerConfig is a minimal subset of the devcontainer.json spec.
type DevcontainerConfig struct {
	Name             string            `json:"name"`
	Build            BuildConfig       `json:"build"`
	Image            string            `json:"image"`
	RunArgs          []string          `json:"runArgs"`
	Mounts           []string          `json:"mounts"`
	WorkspaceMount   string            `json:"workspaceMount"`
	WorkspaceFolder  string            `json:"workspaceFolder"`
	RemoteUser       string            `json:"remoteUser"`
	ContainerEnv     map[string]string `json:"containerEnv"`
	PostStartCommand string            `json:"postStartCommand"`
	WaitFor          string            `json:"waitFor"`
}

type BuildConfig struct {
	Dockerfile string            `json:"dockerfile"`
	Context    string            `json:"context"`
	Args       map[string]string `json:"args"`
}

// PortMapping is a port to publish from container to host.
type PortMapping struct {
	HostPort      int
	ContainerPort int
}

// SpawnOptions configures a devcontainer spawn.
type SpawnOptions struct {
	WorktreePath string
	// SourceRepoPath is the host path of the parent git repo that owns the
	// worktree. When set, the parent repo's .git directory is bind-mounted
	// at the same absolute host path so the worktree's gitdir pointer
	// resolves and `git` works inside the container.
	SourceRepoPath string
	ContainerName  string
	Ports          []PortMapping
}

// SpawnResult is what we return after starting a devcontainer.
type SpawnResult struct {
	ContainerID   string
	ContainerName string
}

var varRE = regexp.MustCompile(`\$\{([^}]+)\}`)

// SubstitutionContext provides the inputs that devcontainer.json variable
// references resolve against. See https://containers.dev/implementors/spec/#variable-substitution.
type SubstitutionContext struct {
	LocalWorkspaceFolder     string
	ContainerWorkspaceFolder string
	DevcontainerID           string
}

// NewSubstitutionContext builds a context for the given worktree and the
// container-side workspace folder (defaults to /workspace if empty).
func NewSubstitutionContext(worktreePath, containerWorkspaceFolder string) SubstitutionContext {
	if containerWorkspaceFolder == "" {
		containerWorkspaceFolder = "/workspace"
	}
	sum := sha256.Sum256([]byte(worktreePath))
	return SubstitutionContext{
		LocalWorkspaceFolder:     worktreePath,
		ContainerWorkspaceFolder: containerWorkspaceFolder,
		DevcontainerID:           hex.EncodeToString(sum[:]),
	}
}

// Substitute resolves devcontainer.json variable references in s per the
// containers.dev spec. Supports ${localEnv:VAR[:default]}, ${localWorkspaceFolder},
// ${localWorkspaceFolderBasename}, ${containerWorkspaceFolder},
// ${containerWorkspaceFolderBasename}, and ${devcontainerId}.
// ${containerEnv:VAR[:default]} references are left literal: they are intended
// to be resolved at runtime inside the container, after it has started.
func Substitute(s string, ctx SubstitutionContext) string {
	return varRE.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[2 : len(match)-1]
		switch inner {
		case "localWorkspaceFolder":
			return ctx.LocalWorkspaceFolder
		case "localWorkspaceFolderBasename":
			return filepath.Base(ctx.LocalWorkspaceFolder)
		case "containerWorkspaceFolder":
			return ctx.ContainerWorkspaceFolder
		case "containerWorkspaceFolderBasename":
			return filepath.Base(ctx.ContainerWorkspaceFolder)
		case "devcontainerId":
			return ctx.DevcontainerID
		}
		if rest, ok := strings.CutPrefix(inner, "localEnv:"); ok {
			name, def := splitDefault(rest)
			if v, ok := os.LookupEnv(name); ok {
				return v
			}
			return def
		}
		// containerEnv:* and any unknown variable are left literal.
		return match
	})
}

// splitDefault splits "VAR:default" into ("VAR", "default"). With no colon,
// returns (s, "").
func splitDefault(s string) (string, string) {
	name, def, _ := strings.Cut(s, ":")
	return name, def
}

// Substitute resolves variable references in every string field of the config
// in place, per the containers.dev spec.
func (c *DevcontainerConfig) Substitute(ctx SubstitutionContext) {
	c.Name = Substitute(c.Name, ctx)
	c.Image = Substitute(c.Image, ctx)
	c.Build.Dockerfile = Substitute(c.Build.Dockerfile, ctx)
	c.Build.Context = Substitute(c.Build.Context, ctx)
	for k, v := range c.Build.Args {
		c.Build.Args[k] = Substitute(v, ctx)
	}
	for i, a := range c.RunArgs {
		c.RunArgs[i] = Substitute(a, ctx)
	}
	for i, m := range c.Mounts {
		c.Mounts[i] = Substitute(m, ctx)
	}
	c.WorkspaceMount = Substitute(c.WorkspaceMount, ctx)
	c.WorkspaceFolder = Substitute(c.WorkspaceFolder, ctx)
	c.RemoteUser = Substitute(c.RemoteUser, ctx)
	for k, v := range c.ContainerEnv {
		c.ContainerEnv[k] = Substitute(v, ctx)
	}
	c.PostStartCommand = Substitute(c.PostStartCommand, ctx)
	c.WaitFor = Substitute(c.WaitFor, ctx)
}

// LoadDevcontainer parses .devcontainer/devcontainer.json from a worktree.
func LoadDevcontainer(worktreePath string) (*DevcontainerConfig, error) {
	path := filepath.Join(worktreePath, ".devcontainer", "devcontainer.json")
	data, err := os.ReadFile(path)
	if err != nil {
		// Fall back to .devcontainer.json at repo root.
		alt := filepath.Join(worktreePath, ".devcontainer.json")
		data, err = os.ReadFile(alt)
		if err != nil {
			return nil, fmt.Errorf("read devcontainer.json: %w", err)
		}
	}
	stripped := stripJSONComments(data)
	var cfg DevcontainerConfig
	if err := json.Unmarshal(stripped, &cfg); err != nil {
		return nil, fmt.Errorf("parse devcontainer.json: %w", err)
	}
	return &cfg, nil
}

// stripJSONComments removes // line comments and /* */ block comments.
// devcontainer.json is "JSON with comments" in practice.
func stripJSONComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escape := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inString {
			out = append(out, c)
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}
		if c == '/' && i+1 < len(data) {
			if data[i+1] == '/' {
				for i < len(data) && data[i] != '\n' {
					i++
				}
				if i < len(data) {
					out = append(out, data[i])
				}
				continue
			}
			if data[i+1] == '*' {
				i += 2
				for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
					i++
				}
				i++
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// Spawn builds and runs the devcontainer for a given worktree.
func (c *Client) Spawn(ctx context.Context, cfg *DevcontainerConfig, opts SpawnOptions) (*SpawnResult, error) {
	if cfg.WorkspaceFolder == "" {
		cfg.WorkspaceFolder = "/workspace"
	}
	cfg.Substitute(NewSubstitutionContext(opts.WorktreePath, cfg.WorkspaceFolder))

	imageRef, err := c.ensureImage(ctx, cfg, opts.WorktreePath)
	if err != nil {
		return nil, fmt.Errorf("ensure image: %w", err)
	}

	hostCfg, netCfg, containerCfg, err := buildContainerConfig(cfg, opts, imageRef)
	if err != nil {
		return nil, err
	}

	created, err := c.cli.ContainerCreate(ctx, containerCfg, hostCfg, netCfg, nil, opts.ContainerName)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	if err := c.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}

	if cfg.PostStartCommand != "" {
		if _, err := c.ExecRun(ctx, created.ID, []string{"sh", "-lc", cfg.PostStartCommand}); err != nil {
			return nil, fmt.Errorf("postStartCommand: %w", err)
		}
	}

	return &SpawnResult{ContainerID: created.ID, ContainerName: opts.ContainerName}, nil
}

func buildContainerConfig(cfg *DevcontainerConfig, opts SpawnOptions, imageRef string) (*container.HostConfig, *network.NetworkingConfig, *container.Config, error) {
	wsFolder := cfg.WorkspaceFolder

	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: opts.WorktreePath, Target: wsFolder, Consistency: mount.ConsistencyDelegated},
	}
	if gitDir, ok := resolveSourceGitDir(opts.SourceRepoPath); ok {
		mounts = append(mounts, mount.Mount{
			Type:        mount.TypeBind,
			Source:      gitDir,
			Target:      gitDir,
			Consistency: mount.ConsistencyDelegated,
		})
	}
	for _, raw := range cfg.Mounts {
		m, err := parseMountString(raw)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse mount %q: %w", raw, err)
		}
		mounts = append(mounts, m)
	}

	hostCfg := &container.HostConfig{
		Mounts:        mounts,
		AutoRemove:    false,
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
	}

	// Note: we deliberately do not publish session container ports to the host.
	// The kanban server's PortProxy bridges traffic via `docker exec ... socat`,
	// so the host-side binding lives on the kanban container itself (which
	// reserves the entire configured port range).
	exposed := nat.PortSet{}
	for _, p := range opts.Ports {
		port, err := nat.NewPort("tcp", fmt.Sprintf("%d", p.ContainerPort))
		if err != nil {
			return nil, nil, nil, err
		}
		exposed[port] = struct{}{}
	}

	applyRunArgs(cfg.RunArgs, hostCfg)

	containerCfg := &container.Config{
		Image:        imageRef,
		WorkingDir:   wsFolder,
		Tty:          false,
		AttachStdout: false,
		AttachStderr: false,
		ExposedPorts: exposed,
		User:         cfg.RemoteUser,
		Cmd:          []string{"sh", "-c", "tail -f /dev/null"},
	}
	if len(cfg.ContainerEnv) > 0 {
		env := make([]string, 0, len(cfg.ContainerEnv))
		for k, v := range cfg.ContainerEnv {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		containerCfg.Env = env
	}

	return hostCfg, &network.NetworkingConfig{}, containerCfg, nil
}

// applyRunArgs translates a small subset of `docker run` flags from runArgs.
func applyRunArgs(args []string, hostCfg *container.HostConfig) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--cap-add" && i+1 < len(args):
			hostCfg.CapAdd = append(hostCfg.CapAdd, args[i+1])
			i++
		case strings.HasPrefix(a, "--cap-add="):
			hostCfg.CapAdd = append(hostCfg.CapAdd, strings.TrimPrefix(a, "--cap-add="))
		case a == "--cap-drop" && i+1 < len(args):
			hostCfg.CapDrop = append(hostCfg.CapDrop, args[i+1])
			i++
		case strings.HasPrefix(a, "--cap-drop="):
			hostCfg.CapDrop = append(hostCfg.CapDrop, strings.TrimPrefix(a, "--cap-drop="))
		case a == "--privileged":
			hostCfg.Privileged = true
		case a == "--network" && i+1 < len(args):
			hostCfg.NetworkMode = container.NetworkMode(args[i+1])
			i++
		case strings.HasPrefix(a, "--network="):
			hostCfg.NetworkMode = container.NetworkMode(strings.TrimPrefix(a, "--network="))
		case a == "--init":
			t := true
			hostCfg.Init = &t
		}
	}
}

// resolveSourceGitDir returns the absolute path of the parent repo's gitdir
// when it can be bind-mounted as-is. Returns ok=false when SourceRepoPath is
// empty, missing, or .git is not a plain directory (e.g., the source itself
// is a linked worktree — handle that case manually).
func resolveSourceGitDir(repoPath string) (string, bool) {
	if repoPath == "" {
		return "", false
	}
	candidate := filepath.Join(repoPath, ".git")
	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return candidate, true
}

func parseMountString(s string) (mount.Mount, error) {
	m := mount.Mount{Type: mount.TypeBind}
	for _, part := range strings.Split(s, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		switch key {
		case "source", "src":
			m.Source = val
		case "target", "destination", "dst":
			m.Target = val
		case "type":
			m.Type = mount.Type(val)
		case "consistency":
			m.Consistency = mount.Consistency(val)
		case "readonly", "ro":
			if val == "true" || val == "1" {
				m.ReadOnly = true
			}
		}
	}
	if m.Target == "" {
		return m, fmt.Errorf("mount missing target")
	}
	if m.Type == mount.TypeBind && m.Source == "" {
		return m, fmt.Errorf("bind mount has empty source (an unset ${localEnv:VAR} likely substituted to empty)")
	}
	return m, nil
}

func (c *Client) ensureImage(ctx context.Context, cfg *DevcontainerConfig, worktreePath string) (string, error) {
	if cfg.Image != "" {
		if _, _, err := c.cli.ImageInspectWithRaw(ctx, cfg.Image); err == nil {
			return cfg.Image, nil
		}
		rc, err := c.cli.ImagePull(ctx, cfg.Image, image.PullOptions{})
		if err != nil {
			return "", fmt.Errorf("pull image %q: %w", cfg.Image, err)
		}
		defer rc.Close()
		if err := jsonmessage.DisplayJSONMessagesStream(rc, io.Discard, 0, false, nil); err != nil {
			return "", fmt.Errorf("pull image %q: %w", cfg.Image, err)
		}
		return cfg.Image, nil
	}
	dockerfileRel := cfg.Build.Dockerfile
	if dockerfileRel == "" {
		dockerfileRel = "Dockerfile"
	}
	contextRel := cfg.Build.Context
	if contextRel == "" {
		contextRel = "."
	}
	contextDir := filepath.Join(worktreePath, ".devcontainer", contextRel)
	if _, err := os.Stat(contextDir); err != nil {
		contextDir = filepath.Join(worktreePath, contextRel)
	}
	dockerfilePath := filepath.Join(worktreePath, ".devcontainer", dockerfileRel)
	if _, err := os.Stat(dockerfilePath); err != nil {
		dockerfilePath = filepath.Join(worktreePath, dockerfileRel)
	}

	tag, err := imageTag(worktreePath, dockerfilePath, cfg.Build.Args)
	if err != nil {
		return "", err
	}
	if _, _, err := c.cli.ImageInspectWithRaw(ctx, tag); err == nil {
		return tag, nil
	}

	tarball, err := buildBuildContext(contextDir, dockerfilePath)
	if err != nil {
		return "", fmt.Errorf("build context: %w", err)
	}

	buildArgs := map[string]*string{}
	for k, v := range cfg.Build.Args {
		val := v
		buildArgs[k] = &val
	}

	resp, err := c.cli.ImageBuild(ctx, bytes.NewReader(tarball), types.ImageBuildOptions{
		Tags:       []string{tag},
		Dockerfile: filepath.Base(dockerfilePath),
		BuildArgs:  buildArgs,
		Remove:     true,
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, io.Discard, 0, false, nil); err != nil {
		return "", fmt.Errorf("image build: %w", err)
	}
	return tag, nil
}

func imageTag(worktreePath, dockerfilePath string, args map[string]string) (string, error) {
	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(data)
	for k, v := range args {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write([]byte(v))
	}
	digest := hex.EncodeToString(h.Sum(nil))[:12]
	base := filepath.Base(worktreePath)
	return fmt.Sprintf("kanban-%s:%s", sanitizeTag(base), digest), nil
}

var tagSafe = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

func sanitizeTag(s string) string {
	return strings.ToLower(tagSafe.ReplaceAllString(s, "-"))
}

// buildBuildContext tars a directory plus a Dockerfile and returns the bytes.
func buildBuildContext(contextDir, dockerfilePath string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	dockerfileName := filepath.Base(dockerfilePath)

	err := filepath.Walk(contextDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(contextDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Ensure Dockerfile is in the tarball even if it sits outside the context dir.
	if _, statErr := os.Stat(filepath.Join(contextDir, dockerfileName)); statErr != nil {
		data, err := os.ReadFile(dockerfilePath)
		if err != nil {
			return nil, err
		}
		hdr := &tar.Header{Name: dockerfileName, Mode: 0o644, Size: int64(len(data)), ModTime: time.Now()}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Exec creates and starts an exec; non-attached, returns exec ID.
func (c *Client) Exec(ctx context.Context, containerID string, cmd []string, workDir string, env []string) (string, error) {
	resp, err := c.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   workDir,
		Env:          env,
	})
	if err != nil {
		return "", err
	}
	if err := c.cli.ContainerExecStart(ctx, resp.ID, container.ExecStartOptions{}); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// ExecRun runs a command synchronously, returning the combined output.
// Returns an error if the exec exits with a non-zero status.
func (c *Client) ExecRun(ctx context.Context, containerID string, cmd []string) (string, error) {
	resp, err := c.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", err
	}
	att, err := c.cli.ContainerExecAttach(ctx, resp.ID, container.ExecStartOptions{})
	if err != nil {
		return "", err
	}
	defer att.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, att.Reader)
	out := buf.String()
	inspect, err := c.cli.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		return out, err
	}
	if inspect.ExitCode != 0 {
		return out, fmt.Errorf("exec exited %d: %s", inspect.ExitCode, strings.TrimSpace(out))
	}
	return out, nil
}

// ExecAttachTTY creates a TTY exec and attaches stdio; the caller pipes IO.
type AttachedExec struct {
	ID   string
	Conn types.HijackedResponse
}

func (c *Client) ExecAttachTTY(ctx context.Context, containerID string, cmd []string, workDir string, env []string) (*AttachedExec, error) {
	resp, err := c.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		WorkingDir:   workDir,
		Env:          env,
	})
	if err != nil {
		return nil, err
	}
	att, err := c.cli.ContainerExecAttach(ctx, resp.ID, container.ExecStartOptions{Tty: true})
	if err != nil {
		return nil, err
	}
	return &AttachedExec{ID: resp.ID, Conn: att}, nil
}

func (c *Client) ResizeExec(ctx context.Context, execID string, cols, rows uint) error {
	return c.cli.ContainerExecResize(ctx, execID, container.ResizeOptions{Width: cols, Height: rows})
}

func (c *Client) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	t := int(timeout.Seconds())
	return c.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &t})
}

func (c *Client) RemoveContainer(ctx context.Context, id string) error {
	return c.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}
