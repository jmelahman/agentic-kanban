package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jmelahman/kanban/internal/client"
)

// addClientCommands attaches the user-facing CLI subcommands (`list-boards`,
// `create-ticket`) to root. They're thin wrappers over the kanban HTTP API,
// useful when kanban is installed natively. For dockerized deploys curl
// against the REST API is friendlier (no host install).
func addClientCommands(root *cobra.Command) {
	var serverURL string
	resolveURL := func(cmd *cobra.Command) string {
		if env := os.Getenv("KANBAN_URL"); env != "" && !cmd.Flags().Changed("server") {
			return env
		}
		return serverURL
	}
	withServerFlag := func(c *cobra.Command) *cobra.Command {
		c.Flags().StringVar(&serverURL, "server", "http://localhost:7474", "Base URL of the kanban HTTP server")
		return c
	}

	listBoards := withServerFlag(&cobra.Command{
		Use:   "list-boards",
		Short: "List kanban boards as a table",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListBoards(cmd.Context(), resolveURL(cmd), cmd.OutOrStdout())
		},
	})

	var (
		ctBoard  string
		ctTitle  string
		ctBody   string
		ctColumn string
		ctJSON   bool
	)
	createTicket := withServerFlag(&cobra.Command{
		Use:   "create-ticket",
		Short: "Create a ticket on a kanban board",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateTicket(cmd.Context(), resolveURL(cmd), cmd.OutOrStdout(),
				client.CreateTicketArgs{Board: ctBoard, Title: ctTitle, Body: ctBody, Column: ctColumn},
				ctJSON,
			)
		},
	})
	createTicket.Flags().StringVar(&ctBoard, "board", "", "Board id or slug (required)")
	createTicket.Flags().StringVar(&ctTitle, "title", "", "Ticket title (required)")
	createTicket.Flags().StringVar(&ctBody, "body", "", "Ticket body / description")
	createTicket.Flags().StringVar(&ctColumn, "column", "", "Column name or id (default: leftmost column)")
	createTicket.Flags().BoolVar(&ctJSON, "json", false, "Print the full ticket JSON instead of a one-line summary")
	_ = createTicket.MarkFlagRequired("board")
	_ = createTicket.MarkFlagRequired("title")

	root.AddCommand(listBoards, createTicket)
}

func runListBoards(ctx context.Context, url string, out io.Writer) error {
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

func runCreateTicket(ctx context.Context, url string, out io.Writer, args client.CreateTicketArgs, asJSON bool) error {
	raw, err := client.New(url, nil).CreateTicket(ctx, args)
	if err != nil {
		return err
	}
	if asJSON {
		fmt.Fprintln(out, string(raw))
		return nil
	}
	var t struct {
		ID    int64  `json:"id"`
		Slug  string `json:"slug"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		fmt.Fprintln(out, string(raw))
		return nil
	}
	fmt.Fprintf(out, "#%d %s (%s)\n", t.ID, t.Title, t.Slug)
	return nil
}
