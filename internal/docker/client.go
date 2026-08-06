package docker

import (
	"context"
	"strings"
	"sync"

	"github.com/docker/docker/client"
)

type Client struct {
	cli *client.Client

	rootlessOnce sync.Once
	rootless     bool
}

func NewClient() (*Client, error) {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Client{cli: c}, nil
}

func (c *Client) Close() error { return c.cli.Close() }

func (c *Client) Raw() *client.Client { return c.cli }

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// Rootless reports whether the daemon runs in rootless mode (cached after
// the first call). Under rootless docker, root inside a container maps to
// the daemon's host user while other uids map to subordinate ids — callers
// that care about bind-mount file ownership need to know.
func (c *Client) Rootless(ctx context.Context) bool {
	c.rootlessOnce.Do(func() {
		info, err := c.cli.Info(ctx)
		if err != nil {
			return
		}
		for _, opt := range info.SecurityOptions {
			if strings.Contains(opt, "rootless") {
				c.rootless = true
				return
			}
		}
	})
	return c.rootless
}
