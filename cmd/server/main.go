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
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/jmelahman/kanban/internal/api"
	"github.com/jmelahman/kanban/internal/config"
	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/github"
	"github.com/jmelahman/kanban/internal/hooks"
	"github.com/jmelahman/kanban/internal/session"
)

// Build metadata. These are populated at build time via -ldflags -X. The
// defaults make `go run` and dev builds explicit instead of looking like a
// release.
var (
	version = "dev"
	commit  = "none"
	dirty   = "false"
)

// BuildInfo describes the running binary. dirty is true when the source tree
// had uncommitted changes at build time.
type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Dirty   bool   `json:"dirty"`
}

// Build returns the build metadata for the running binary.
func Build() BuildInfo {
	return BuildInfo{Version: version, Commit: commit, Dirty: dirty == "true" || dirty == "1"}
}

func Root() *cobra.Command {
	var addr string
	var dataDir string
	var portRangeStart int
	var portRangeEnd int

	commitLabel := commit
	if Build().Dirty {
		commitLabel += "-dirty"
	}
	cmd := &cobra.Command{
		Use:     "kanban",
		Short:   "Kanban board for managing AI agent sessions",
		Version: fmt.Sprintf("%s\ncommit %s", version, commitLabel),
	}

	serve := &cobra.Command{
		Use:   "serve",
		Short: "Start the kanban HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(addr, dataDir, portRangeStart, portRangeEnd)
		},
	}
	serve.Flags().StringVar(&addr, "addr", ":7474", "HTTP listen address")
	serve.Flags().StringVar(&dataDir, "data-dir", "", "Override data directory (default: $KANBAN_DATA_DIR or XDG)")
	serve.Flags().IntVar(&portRangeStart, "port-range-start", 13000, "First host port available for proxy allocation")
	serve.Flags().IntVar(&portRangeEnd, "port-range-end", 13099, "Last host port available for proxy allocation (inclusive)")

	cmd.AddCommand(serve)
	return cmd
}

func run(addr, dataDirOverride string, portStart, portEnd int) error {
	cfg, err := config.Load(dataDirOverride, portStart, portEnd)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	store, err := db.Open(cfg.DBPath())
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
	apiBase := buildAPIBase(selfName, addr)
	// Log the resolved callback URL so host-mode runs (selfName == "") are easy
	// to spot: "host.docker.internal" only resolves under Docker Desktop or
	// when the session container has --add-host=host-gateway, which is why
	// status hooks tend to silently fail on bare-metal Linux setups.
	log.Printf("session callback api base: %s (self container=%q)", apiBase, selfName)
	sessionMgr.SetAPIBase(apiBase)

	bus := api.NewEventBus()

	mux := api.NewMux(api.Deps{
		Store:    store,
		Docker:   dockerClient,
		Sessions: sessionMgr,
		Hooks:    hookRunner,
		Config:   cfg,
		Bus:      bus,
		Build: api.BuildInfo{
			Version: version,
			Commit:  commit,
			Dirty:   dirty == "true" || dirty == "1",
		},
	})

	pollerCtx, pollerCancel := context.WithCancel(context.Background())
	defer pollerCancel()
	go github.NewPoller(store, bus, sessionMgr, 30*time.Second).Start(pollerCtx)

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux),
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

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		h.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}
