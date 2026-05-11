package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/jmelahman/kanban/internal/api"
	"github.com/jmelahman/kanban/internal/config"
	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/errreport"
	"github.com/jmelahman/kanban/internal/github"
	"github.com/jmelahman/kanban/internal/hooks"
	"github.com/jmelahman/kanban/internal/mcp"
	"github.com/jmelahman/kanban/internal/session"
)

// version is populated at build time via -ldflags -X (see Dockerfile /
// compose.yaml). Use `git describe --tags --always --dirty` so a single
// string carries tag, distance, short sha, and dirty marker. Falls back to
// runtime/debug VCS info for dev builds without ldflags.
var version = ""

// BuildInfo describes the running binary.
type BuildInfo struct {
	Version string `json:"version"`
}

// Build returns the build metadata for the running binary, falling back to
// runtime/debug VCS info when ldflags weren't set.
func Build() BuildInfo {
	if version != "" {
		return BuildInfo{Version: version}
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		var rev string
		var modified bool
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				modified = s.Value == "true"
			}
		}
		if rev != "" {
			short := rev
			if len(short) > 7 {
				short = short[:7]
			}
			if modified {
				short += "-dirty"
			}
			return BuildInfo{Version: short}
		}
	}
	return BuildInfo{Version: "dev"}
}

func Root() *cobra.Command {
	var addr string
	var dataDir string
	var worktreesDir string
	var configPath string
	var portRangeStart int
	var portRangeEnd int
	var inMemory bool
	var claudeConfig bool

	cmd := &cobra.Command{
		Use:     "kanban",
		Short:   "Kanban board for managing AI agent sessions",
		Version: Build().Version,
	}

	serve := &cobra.Command{
		Use:   "serve",
		Short: "Start the kanban HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := applyConfigPath(configPath); err != nil {
				return err
			}
			override := resolveClaudeConfigOverride(cmd, claudeConfig)
			return run(addr, dataDir, worktreesDir, portRangeStart, portRangeEnd, inMemory, override)
		},
	}
	serve.Flags().StringVar(&addr, "addr", ":7474", "HTTP listen address")
	serve.Flags().StringVar(&dataDir, "data-dir", "", "Override data directory (default: $KANBAN_DATA_DIR or XDG)")
	serve.Flags().StringVar(&worktreesDir, "worktrees-dir", "", "Override default parent directory for new board worktrees (default: $KANBAN_WORKTREES_DIR or <data-dir>/worktrees)")
	serve.Flags().StringVar(&configPath, "config", "", "Override user-level kanban config path (default: $KANBAN_CONFIG or $XDG_CONFIG_HOME/kanban/config.toml)")
	serve.Flags().IntVar(&portRangeStart, "port-range-start", 13000, "First host port available for proxy allocation")
	serve.Flags().IntVar(&portRangeEnd, "port-range-end", 13099, "Last host port available for proxy allocation (inclusive)")
	serve.Flags().BoolVar(&inMemory, "in-memory", false, "Use an ephemeral in-memory SQLite database; all data is discarded on shutdown")
	serve.Flags().BoolVar(&claudeConfig, "claude-config", true, "Forward host Claude Code config (~/.claude, ~/.claude.json) into built-in session containers. When set explicitly, overrides .kanban.toml [devcontainer].claude_config; otherwise the toml setting wins. Env: $KANBAN_CLAUDE_CONFIG.")

	cmd.AddCommand(serve)

	var mcpServerURL string
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run kanban as an MCP (Model Context Protocol) server over stdio",
		Long: "Run a Model Context Protocol server on stdin/stdout that exposes\n" +
			"kanban tools to AI agents (Claude Desktop, Claude Code, etc.).\n" +
			"Tools forward to a running `kanban serve` over HTTP.",
		RunE: func(cmd *cobra.Command, args []string) error {
			url := mcpServerURL
			if env := os.Getenv("KANBAN_URL"); env != "" && !cmd.Flags().Changed("server") {
				url = env
			}
			return mcp.Run(cmd.Context(), url)
		},
	}
	mcpCmd.Flags().StringVar(&mcpServerURL, "server", "http://localhost:7474", "Base URL of the kanban HTTP server")
	cmd.AddCommand(mcpCmd)

	addClientCommands(cmd)

	return cmd
}

// applyConfigPath publishes the requested user-config path via $KANBAN_CONFIG
// so kanbantoml.UserPath() (called from many sites) picks it up without
// threading the override through every caller. The flag wins over a
// pre-existing env var; an empty flag leaves the env alone.
func applyConfigPath(p string) error {
	if p == "" {
		return nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return fmt.Errorf("resolve --config path: %w", err)
	}
	return os.Setenv("KANBAN_CONFIG", abs)
}

// resolveClaudeConfigOverride collapses --claude-config and $KANBAN_CLAUDE_CONFIG
// into the *bool the session manager expects: nil means "defer to .kanban.toml",
// non-nil means "force this value regardless of toml".
func resolveClaudeConfigOverride(cmd *cobra.Command, flagVal bool) *bool {
	if cmd.Flags().Changed("claude-config") {
		v := flagVal
		return &v
	}
	if env := os.Getenv("KANBAN_CLAUDE_CONFIG"); env != "" {
		if v, err := strconv.ParseBool(env); err == nil {
			return &v
		}
	}
	return nil
}

func run(addr, dataDirOverride, worktreesDirOverride string, portStart, portEnd int, inMemory bool, claudeConfigOverride *bool) error {
	cfg, err := config.Load(dataDirOverride, worktreesDirOverride, portStart, portEnd)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbPath := cfg.DBPath()
	if inMemory {
		log.Printf("WARNING: --in-memory set; using ephemeral SQLite, all data is lost on shutdown")
		dbPath = ":memory:"
	}
	store, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()

	dockerClient, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	defer dockerClient.Close()

	// Ensure a shared docker network so session containers can resolve and
	// reach the kanban API by container name. Failures here are non-fatal:
	// session→kanban callbacks (status updates) just won't work. Each docker
	// call gets its own timeout so a slow daemon on one step can't starve the
	// next.
	ensureCtx, ensureCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := dockerClient.EnsureNetwork(ensureCtx, docker.KanbanNetworkName); err != nil {
		log.Printf("ensure network %s: %v", docker.KanbanNetworkName, err)
	}
	ensureCancel()

	selfCtx, selfCancel := context.WithTimeout(context.Background(), 10*time.Second)
	selfName := dockerClient.SelfContainerName(selfCtx)
	selfCancel()

	if selfName != "" {
		connCtx, connCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := dockerClient.ConnectContainer(connCtx, docker.KanbanNetworkName, selfName); err != nil {
			log.Printf("connect kanban to %s network: %v", docker.KanbanNetworkName, err)
		}
		connCancel()
	}

	hookRunner := hooks.NewRunner(store)
	sessionMgr := session.NewManager(store, dockerClient, hookRunner)
	sessionMgr.SetClaudeConfigOverride(claudeConfigOverride)
	apiBase := buildAPIBase(selfName, addr)
	// Log the resolved callback URL so host-mode runs (selfName == "") are easy
	// to spot: "host.docker.internal" only resolves under Docker Desktop or
	// when the session container has --add-host=host-gateway, which is why
	// status hooks tend to silently fail on bare-metal Linux setups.
	log.Printf("session callback api base: %s (self container=%q)", apiBase, selfName)
	sessionMgr.SetAPIBase(apiBase)

	bus := api.NewEventBus()

	errCfg := errreport.ResolveConfig("")
	reporter := errreport.New(store, errCfg)
	if errCfg.Enabled {
		log.Printf("error-reporting enabled, board=%q", errCfg.BoardName)
	}

	mux := api.NewMux(api.Deps{
		Store:    store,
		Docker:   dockerClient,
		Sessions: sessionMgr,
		Hooks:    hookRunner,
		Config:   cfg,
		Bus:      bus,
		Build:    api.BuildInfo(Build()),
		Reporter: reporter,
	})

	pollerCtx, pollerCancel := context.WithCancel(context.Background())
	defer pollerCancel()
	go github.NewPoller(store, bus, sessionMgr, 30*time.Second).Start(pollerCtx)

	// h2c lets clients that opt in (curl --http2-prior-knowledge, Go's
	// http2.Transport, or a TLS-terminating proxy upstream) multiplex over
	// one connection. Browsers don't negotiate HTTP/2 over plain HTTP, so the
	// web frontend stays on HTTP/1.1 unless fronted by a TLS proxy.
	h2s := &http2.Server{}
	srv := &http.Server{
		Addr:              addr,
		Handler:           h2c.NewHandler(recoverPanics(reporter, logRequests(compressResponses(mux))), h2s),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("kanban listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijack not supported")
	}
	return hj.Hijack()
}

// buildAPIBase returns the base URL session containers should use to call the
// kanban API. When kanban runs in a container, sessions resolve it by name on
// the shared docker network. Outside a container we fall back to
// host.docker.internal so Docker Desktop / host-gateway setups still work.
func buildAPIBase(selfName, addr string) string {
	port := "7474"
	if _, p, err := net.SplitHostPort(addr); err == nil && p != "" {
		port = p
	}
	host := selfName
	if host == "" {
		host = "host.docker.internal"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

// recoverPanics is the outermost middleware. It catches panics from any
// downstream handler so the process survives, logs the trace, files a ticket
// via the reporter (no-op when disabled), and writes a 500 response. The
// panic-survival behavior is unconditional; only ticket creation is gated by
// the reporter being enabled.
func recoverPanics(reporter *errreport.Reporter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			stack := string(debug.Stack())
			log.Printf("panic recovered: %v\n%s", rec, stack)
			reporter.Report(r.Context(), "panic",
				fmt.Sprintf("panic: %v", rec), stack,
				map[string]string{"path": r.URL.Path, "method": r.Method})
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		h.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}
