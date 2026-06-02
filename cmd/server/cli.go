package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jmelahman/kanban/internal/client"
)

// addClientCommands attaches the user-facing CLI subcommand groups
// (`board`, `ticket`, `column`, `session`) to root. They're thin wrappers
// over the kanban HTTP API, useful for scripting and remote control of a
// running `kanban serve`. For dockerized deploys curl against the REST API
// is friendlier (no host install).
func addClientCommands(root *cobra.Command) {
	root.AddCommand(boardCmd(), ticketCmd(), columnCmd(), sessionCmd(), configCmd())
}

// resolveURL returns the effective server URL for a leaf command. KANBAN_URL
// wins only when the user didn't explicitly pass --server.
func resolveURL(cmd *cobra.Command, serverURL string) string {
	if env := os.Getenv("KANBAN_URL"); env != "" && !cmd.Flags().Changed("server") {
		return env
	}
	return serverURL
}

// addServerFlag registers --server as a persistent flag on the parent group
// so every leaf inherits it. The same string variable is reused across the
// group's subcommands.
func addServerFlag(parent *cobra.Command, dst *string) {
	parent.PersistentFlags().StringVar(dst, "server", "http://localhost:7474",
		"Base URL of the kanban HTTP server")
}

// ---------- board ----------

func boardCmd() *cobra.Command {
	var serverURL string
	parent := &cobra.Command{
		Use:   "board",
		Short: "Manage kanban boards",
	}
	addServerFlag(parent, &serverURL)

	list := &cobra.Command{
		Use:   "list",
		Short: "List kanban boards as a table",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoardList(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout())
		},
	}

	var (
		bcName, bcRepo, bcMount, bcWorktreeRoot, bcBaseBranch, bcBranchPrefix string
		bcJSON                                                                bool
	)
	create := &cobra.Command{
		Use:   "create",
		Short: "Create a new board",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoardCreate(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(),
				client.CreateBoardArgs{
					Name:         bcName,
					RepoPath:     bcRepo,
					MountPath:    bcMount,
					WorktreeRoot: bcWorktreeRoot,
					BaseBranch:   bcBaseBranch,
					BranchPrefix: bcBranchPrefix,
				}, bcJSON)
		},
	}
	create.Flags().StringVar(&bcName, "name", "", "Board name (required)")
	create.Flags().StringVar(&bcRepo, "repo-path", "", "Path to the git repo on the host (required if --mount-path is empty)")
	create.Flags().StringVar(&bcMount, "mount-path", "", "Mount path inside session containers (alternative to --repo-path)")
	create.Flags().StringVar(&bcWorktreeRoot, "worktree-root", "", "Override the parent directory for new session worktrees")
	create.Flags().StringVar(&bcBaseBranch, "base-branch", "", "Branch session worktrees fork from (default: main)")
	create.Flags().StringVar(&bcBranchPrefix, "branch-prefix", "", "Optional prefix prepended to session branch names")
	create.Flags().BoolVar(&bcJSON, "json", false, "Print the full board JSON instead of a one-line summary")
	_ = create.MarkFlagRequired("name")

	var bgJSON bool
	get := &cobra.Command{
		Use:   "get <id>",
		Short: "Print a single board",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoardGet(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), args[0], bgJSON)
		},
	}
	get.Flags().BoolVar(&bgJSON, "json", false, "Print the full board JSON instead of a one-line summary")

	var (
		buName, buRepo, buMount, buWorktreeRoot, buBaseBranch, buBranchPrefix string
		buJSON                                                                bool
	)
	update := &cobra.Command{
		Use:   "update <id>",
		Short: "Update fields on a board",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a := client.UpdateBoardArgs{}
			if cmd.Flags().Changed("name") {
				a.Name = &buName
			}
			if cmd.Flags().Changed("repo-path") {
				a.RepoPath = &buRepo
			}
			if cmd.Flags().Changed("mount-path") {
				a.MountPath = &buMount
			}
			if cmd.Flags().Changed("worktree-root") {
				a.WorktreeRoot = &buWorktreeRoot
			}
			if cmd.Flags().Changed("base-branch") {
				a.BaseBranch = &buBaseBranch
			}
			if cmd.Flags().Changed("branch-prefix") {
				a.BranchPrefix = &buBranchPrefix
			}
			return runBoardUpdate(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), args[0], a, buJSON)
		},
	}
	update.Flags().StringVar(&buName, "name", "", "Rename the board")
	update.Flags().StringVar(&buRepo, "repo-path", "", "Update repo path")
	update.Flags().StringVar(&buMount, "mount-path", "", "Update mount path")
	update.Flags().StringVar(&buWorktreeRoot, "worktree-root", "", "Update worktree root")
	update.Flags().StringVar(&buBaseBranch, "base-branch", "", "Update base branch")
	update.Flags().StringVar(&buBranchPrefix, "branch-prefix", "", "Update branch prefix")
	update.Flags().BoolVar(&buJSON, "json", false, "Print the full board JSON instead of a one-line summary")

	del := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a board (and destroy all its sessions)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoardDelete(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), args[0])
		},
	}

	state := &cobra.Command{
		Use:   "state <id>",
		Short: "Print full board state (board, columns, tickets, sessions) as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoardState(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), args[0])
		},
	}

	archived := &cobra.Command{
		Use:   "archived <id>",
		Short: "List archived tickets on a board",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoardArchived(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), args[0])
		},
	}

	archivedClear := &cobra.Command{
		Use:   "archived-clear <id>",
		Short: "Permanently delete every archived ticket on a board",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoardArchivedClear(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), args[0])
		},
	}

	parent.AddCommand(list, create, get, update, del, state, archived, archivedClear)
	return parent
}

func runBoardList(ctx context.Context, url string, out io.Writer) error {
	boards, err := client.New(url, nil).ListBoards(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSLUG\tNAME")
	for _, b := range boards {
		fmt.Fprintf(tw, "%d\t%s\t%s\n", b.ID, b.Slug, b.Name)
	}
	return tw.Flush()
}

func runBoardCreate(ctx context.Context, url string, out io.Writer, args client.CreateBoardArgs, asJSON bool) error {
	raw, err := client.New(url, nil).CreateBoard(ctx, args)
	if err != nil {
		return err
	}
	return printBoardSummary(out, raw, asJSON)
}

func runBoardGet(ctx context.Context, url string, out io.Writer, ident string, asJSON bool) error {
	c := client.New(url, nil)
	id, err := c.ResolveBoardID(ctx, ident)
	if err != nil {
		return err
	}
	raw, err := c.GetBoard(ctx, id)
	if err != nil {
		return err
	}
	return printBoardSummary(out, raw, asJSON)
}

func runBoardUpdate(ctx context.Context, url string, out io.Writer, ident string, args client.UpdateBoardArgs, asJSON bool) error {
	c := client.New(url, nil)
	id, err := c.ResolveBoardID(ctx, ident)
	if err != nil {
		return err
	}
	raw, err := c.UpdateBoard(ctx, id, args)
	if err != nil {
		return err
	}
	return printBoardSummary(out, raw, asJSON)
}

func runBoardDelete(ctx context.Context, url string, out io.Writer, ident string) error {
	c := client.New(url, nil)
	id, err := c.ResolveBoardID(ctx, ident)
	if err != nil {
		return err
	}
	if err := c.DeleteBoard(ctx, id); err != nil {
		return err
	}
	fmt.Fprintf(out, "deleted board %d\n", id)
	return nil
}

func runBoardState(ctx context.Context, url string, out io.Writer, ident string) error {
	c := client.New(url, nil)
	id, err := c.ResolveBoardID(ctx, ident)
	if err != nil {
		return err
	}
	raw, err := c.BoardState(ctx, id)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(raw))
	return err
}

func runBoardArchived(ctx context.Context, url string, out io.Writer, ident string) error {
	c := client.New(url, nil)
	id, err := c.ResolveBoardID(ctx, ident)
	if err != nil {
		return err
	}
	raw, err := c.ListArchived(ctx, id)
	if err != nil {
		return err
	}
	var tickets []client.Ticket
	if err := json.Unmarshal(raw, &tickets); err != nil {
		_, perr := fmt.Fprintln(out, string(raw))
		return perr
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSLUG\tTITLE")
	for _, t := range tickets {
		fmt.Fprintf(tw, "%d\t%s\t%s\n", t.ID, t.Slug, t.Title)
	}
	return tw.Flush()
}

func runBoardArchivedClear(ctx context.Context, url string, out io.Writer, ident string) error {
	c := client.New(url, nil)
	id, err := c.ResolveBoardID(ctx, ident)
	if err != nil {
		return err
	}
	if err := c.DeleteArchived(ctx, id); err != nil {
		return err
	}
	fmt.Fprintf(out, "cleared archived tickets on board %d\n", id)
	return nil
}

// printBoardSummary prints either the raw JSON or a one-line summary derived
// from id/slug/name.
func printBoardSummary(out io.Writer, raw json.RawMessage, asJSON bool) error {
	if asJSON {
		_, err := fmt.Fprintln(out, string(raw))
		return err
	}
	var b struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		_, perr := fmt.Fprintln(out, string(raw))
		return perr
	}
	fmt.Fprintf(out, "#%d %s (%s)\n", b.ID, b.Name, b.Slug)
	return nil
}

// ---------- ticket ----------

func ticketCmd() *cobra.Command {
	var serverURL string
	parent := &cobra.Command{
		Use:   "ticket",
		Short: "Manage tickets on a kanban board",
	}
	addServerFlag(parent, &serverURL)

	var (
		tcBoard, tcTitle, tcBody, tcColumn string
		tcJSON                             bool
	)
	create := &cobra.Command{
		Use:   "create",
		Short: "Create a ticket on a board",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTicketCreate(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(),
				client.CreateTicketArgs{Board: tcBoard, Title: tcTitle, Body: tcBody, Column: tcColumn},
				tcJSON,
			)
		},
	}
	create.Flags().StringVar(&tcBoard, "board", "", "Board id or slug (required)")
	create.Flags().StringVar(&tcTitle, "title", "", "Ticket title (required)")
	create.Flags().StringVar(&tcBody, "body", "", "Ticket body / description")
	create.Flags().StringVar(&tcColumn, "column", "", "Column name or id (default: leftmost column)")
	create.Flags().BoolVar(&tcJSON, "json", false, "Print the full ticket JSON instead of a one-line summary")
	_ = create.MarkFlagRequired("board")
	_ = create.MarkFlagRequired("title")

	var (
		tuTitle, tuBody string
		tuJSON          bool
	)
	update := &cobra.Command{
		Use:   "update <id>",
		Short: "Update title/body of a ticket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInt64(args[0], "ticket id")
			if err != nil {
				return err
			}
			a := client.UpdateTicketArgs{}
			if cmd.Flags().Changed("title") {
				a.Title = &tuTitle
			}
			if cmd.Flags().Changed("body") {
				a.Body = &tuBody
			}
			return runTicketUpdate(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), id, a, tuJSON)
		},
	}
	update.Flags().StringVar(&tuTitle, "title", "", "New title")
	update.Flags().StringVar(&tuBody, "body", "", "New body")
	update.Flags().BoolVar(&tuJSON, "json", false, "Print the full ticket JSON instead of a one-line summary")

	var (
		tmColumnID int64
		tmPosition int
	)
	move := &cobra.Command{
		Use:   "move <id>",
		Short: "Move a ticket to a different column / position",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInt64(args[0], "ticket id")
			if err != nil {
				return err
			}
			return runTicketMove(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), id,
				client.MoveTicketArgs{ColumnID: tmColumnID, Position: tmPosition})
		},
	}
	move.Flags().Int64Var(&tmColumnID, "column-id", 0, "Target column id (numeric, required)")
	move.Flags().IntVar(&tmPosition, "position", 0, "Target position within the column (0-indexed)")
	_ = move.MarkFlagRequired("column-id")

	archive := simpleTicketCmd("archive", "Archive a ticket", &serverURL,
		func(c *client.Client, ctx context.Context, id int64) error { return c.ArchiveTicket(ctx, id) })
	unarchive := simpleTicketCmd("unarchive", "Unarchive a ticket", &serverURL,
		func(c *client.Client, ctx context.Context, id int64) error { return c.UnarchiveTicket(ctx, id) })
	delTicket := simpleTicketCmd("delete", "Permanently delete a ticket (must be archived first)", &serverURL,
		func(c *client.Client, ctx context.Context, id int64) error { return c.DeleteTicket(ctx, id) })
	doneCmd := simpleTicketCmd("done", "Move a ticket to the rightmost column and stop its session", &serverURL,
		func(c *client.Client, ctx context.Context, id int64) error { return c.DoneTicket(ctx, id) })

	var syncStrategy string
	sync := &cobra.Command{
		Use:   "sync <id>",
		Short: "Sync a ticket branch from base (rebase|merge)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInt64(args[0], "ticket id")
			if err != nil {
				return err
			}
			return runTicketSync(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), id, syncStrategy)
		},
	}
	sync.Flags().StringVar(&syncStrategy, "strategy", "rebase", "Sync strategy: rebase or merge")

	var mergeStrategy string
	mergeCmd := &cobra.Command{
		Use:   "merge <id>",
		Short: "Merge a ticket branch into base (merge-commit|squash|rebase)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInt64(args[0], "ticket id")
			if err != nil {
				return err
			}
			return runTicketMerge(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), id, mergeStrategy)
		},
	}
	mergeCmd.Flags().StringVar(&mergeStrategy, "strategy", "", "Merge strategy: merge-commit, squash, or rebase (required)")
	_ = mergeCmd.MarkFlagRequired("strategy")

	parent.AddCommand(create, update, move, archive, unarchive, delTicket, doneCmd, sync, mergeCmd)
	return parent
}

func runTicketCreate(ctx context.Context, url string, out io.Writer, args client.CreateTicketArgs, asJSON bool) error {
	raw, err := client.New(url, nil).CreateTicket(ctx, args)
	if err != nil {
		return err
	}
	return printTicketSummary(out, raw, asJSON)
}

func runTicketUpdate(ctx context.Context, url string, out io.Writer, id int64, args client.UpdateTicketArgs, asJSON bool) error {
	raw, err := client.New(url, nil).UpdateTicket(ctx, id, args)
	if err != nil {
		return err
	}
	return printTicketSummary(out, raw, asJSON)
}

func runTicketMove(ctx context.Context, url string, out io.Writer, id int64, args client.MoveTicketArgs) error {
	if err := client.New(url, nil).MoveTicket(ctx, id, args); err != nil {
		return err
	}
	fmt.Fprintf(out, "moved ticket %d to column %d position %d\n", id, args.ColumnID, args.Position)
	return nil
}

func runTicketSync(ctx context.Context, url string, out io.Writer, id int64, strategy string) error {
	if err := client.New(url, nil).SyncTicket(ctx, id, strategy); err != nil {
		return err
	}
	fmt.Fprintf(out, "synced ticket %d (%s)\n", id, strategy)
	return nil
}

func runTicketMerge(ctx context.Context, url string, out io.Writer, id int64, strategy string) error {
	if err := client.New(url, nil).MergeTicket(ctx, id, strategy); err != nil {
		return err
	}
	fmt.Fprintf(out, "merged ticket %d (%s)\n", id, strategy)
	return nil
}

// simpleTicketCmd builds an archive/unarchive/delete/done style subcommand
// (positional ticket id, no body, no flags) that maps to a one-line client
// call. The shape is the same for all four commands; this avoids four
// near-identical blocks in ticketCmd().
func simpleTicketCmd(use, short string, serverURL *string, action func(c *client.Client, ctx context.Context, id int64) error) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInt64(args[0], "ticket id")
			if err != nil {
				return err
			}
			if err := action(client.New(resolveURL(cmd, *serverURL), nil), cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s ticket %d\n", use, id)
			return nil
		},
	}
}

func printTicketSummary(out io.Writer, raw json.RawMessage, asJSON bool) error {
	if asJSON {
		_, err := fmt.Fprintln(out, string(raw))
		return err
	}
	var t struct {
		ID    int64  `json:"id"`
		Slug  string `json:"slug"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		_, perr := fmt.Fprintln(out, string(raw))
		return perr
	}
	fmt.Fprintf(out, "#%d %s (%s)\n", t.ID, t.Title, t.Slug)
	return nil
}

// ---------- column ----------

func columnCmd() *cobra.Command {
	var serverURL string
	parent := &cobra.Command{
		Use:   "column",
		Short: "Column-level operations",
	}
	addServerFlag(parent, &serverURL)

	archiveAll := &cobra.Command{
		Use:   "archive-all <id>",
		Short: "Archive every ticket in a column (numeric column id)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInt64(args[0], "column id")
			if err != nil {
				return err
			}
			return runColumnArchiveAll(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), id)
		},
	}

	parent.AddCommand(archiveAll)
	return parent
}

func runColumnArchiveAll(ctx context.Context, url string, out io.Writer, columnID int64) error {
	if err := client.New(url, nil).ArchiveColumnTickets(ctx, columnID); err != nil {
		return err
	}
	fmt.Fprintf(out, "archived all tickets in column %d\n", columnID)
	return nil
}

// ---------- session ----------

func sessionCmd() *cobra.Command {
	var serverURL string
	parent := &cobra.Command{
		Use:   "session",
		Short: "Manage agent sessions",
	}
	addServerFlag(parent, &serverURL)

	var (
		seTicket int64
		seJSON   bool
	)
	ensure := &cobra.Command{
		Use:   "ensure",
		Short: "Ensure a session exists for a ticket (create if absent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionEnsure(cmd.Context(), resolveURL(cmd, serverURL), cmd.OutOrStdout(), seTicket, seJSON)
		},
	}
	ensure.Flags().Int64Var(&seTicket, "ticket", 0, "Ticket id (required)")
	ensure.Flags().BoolVar(&seJSON, "json", false, "Print the full session JSON instead of a one-line summary")
	_ = ensure.MarkFlagRequired("ticket")

	start := sessionLifecycleCmd("start", "Start a stopped session", &serverURL,
		func(c *client.Client, ctx context.Context, id int64) (json.RawMessage, error) { return c.StartSession(ctx, id) })
	restart := sessionLifecycleCmd("restart", "Restart a session", &serverURL,
		func(c *client.Client, ctx context.Context, id int64) (json.RawMessage, error) { return c.RestartSession(ctx, id) })

	stop := &cobra.Command{
		Use:   "stop <id>",
		Short: "Stop a running session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInt64(args[0], "session id")
			if err != nil {
				return err
			}
			if err := client.New(resolveURL(cmd, serverURL), nil).StopSession(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "stopped session %d\n", id)
			return nil
		},
	}

	parent.AddCommand(ensure, start, stop, restart)
	return parent
}

func runSessionEnsure(ctx context.Context, url string, out io.Writer, ticketID int64, asJSON bool) error {
	raw, err := client.New(url, nil).EnsureSession(ctx, ticketID)
	if err != nil {
		return err
	}
	return printSessionSummary(out, raw, asJSON)
}

// sessionLifecycleCmd builds start/restart subcommands (positional session
// id, --json flag) that return the session body.
func sessionLifecycleCmd(use, short string, serverURL *string, action func(c *client.Client, ctx context.Context, id int64) (json.RawMessage, error)) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInt64(args[0], "session id")
			if err != nil {
				return err
			}
			raw, err := action(client.New(resolveURL(cmd, *serverURL), nil), cmd.Context(), id)
			if err != nil {
				return err
			}
			return printSessionSummary(cmd.OutOrStdout(), raw, asJSON)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "Print the full session JSON instead of a one-line summary")
	return c
}

func printSessionSummary(out io.Writer, raw json.RawMessage, asJSON bool) error {
	if asJSON {
		_, err := fmt.Fprintln(out, string(raw))
		return err
	}
	var s struct {
		ID       int64  `json:"id"`
		TicketID int64  `json:"ticket_id"`
		Status   string `json:"status"`
		Branch   string `json:"branch_name"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		_, perr := fmt.Fprintln(out, string(raw))
		return perr
	}
	fmt.Fprintf(out, "session #%d ticket=%d status=%s branch=%s\n", s.ID, s.TicketID, s.Status, s.Branch)
	return nil
}

// parseInt64 wraps strconv.ParseInt with a descriptive error.
func parseInt64(s, label string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", label, s, err)
	}
	return id, nil
}
