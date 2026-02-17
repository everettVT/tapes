package pushcmder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/tapes/cmd/tapes/sqlitepath"
	"github.com/papercomputeco/tapes/pkg/merkle"
	"github.com/papercomputeco/tapes/pkg/storage/sqlite"
)

const pushLongDesc string = `Push local nodes to a remote tapes server.

Reads all nodes from the local SQLite database and POSTs them
to the remote server's /dag/nodes endpoint. Content-addressing
ensures duplicates are automatically skipped on the server side.

Examples:
  tapes push http://192.168.1.42:6061
  tapes push --sqlite ~/.tapes/tapes.db http://localhost:6061`

const pushShortDesc string = "Push nodes to a remote tapes server"

type pushCommander struct {
	sqlitePath string
	batchSize  int
}

type pushResponse struct {
	New       int `json:"new"`
	Duplicate int `json:"duplicate"`
	Errors    int `json:"errors"`
}

func NewPushCmd() *cobra.Command {
	cmder := &pushCommander{}

	cmd := &cobra.Command{
		Use:   "push <server-url>",
		Short: pushShortDesc,
		Long:  pushLongDesc,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmder.run(cmd.Context(), cmd, args[0])
		},
	}

	cmd.Flags().StringVarP(&cmder.sqlitePath, "sqlite", "s", "", "Path to local SQLite database")
	cmd.Flags().IntVar(&cmder.batchSize, "batch-size", 500, "Nodes per HTTP request")

	return cmd
}

func (c *pushCommander) run(ctx context.Context, cmd *cobra.Command, serverURL string) error {
	serverURL = strings.TrimRight(serverURL, "/")

	dbPath, err := sqlitepath.ResolveSQLitePath(c.sqlitePath)
	if err != nil {
		return fmt.Errorf("could not resolve local database: %w", err)
	}

	driver, err := sqlite.NewDriver(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("could not open local database %s: %w", dbPath, err)
	}
	defer driver.Close()

	nodes, err := driver.List(ctx)
	if err != nil {
		return fmt.Errorf("could not list local nodes: %w", err)
	}

	if len(nodes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No local nodes to push.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushing %d nodes from %s to %s\n", len(nodes), dbPath, serverURL)

	var totalNew, totalDup, totalErr int

	for i := 0; i < len(nodes); i += c.batchSize {
		end := i + c.batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[i:end]

		resp, err := c.postBatch(serverURL, batch)
		if err != nil {
			return fmt.Errorf("push failed on batch %d-%d: %w", i, end-1, err)
		}

		totalNew += resp.New
		totalDup += resp.Duplicate
		totalErr += resp.Errors
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushed %d new nodes (%d already existed, %d errors)\n",
		totalNew, totalDup, totalErr)

	return nil
}

func (c *pushCommander) postBatch(serverURL string, nodes []*merkle.Node) (*pushResponse, error) {
	body, err := json.Marshal(nodes)
	if err != nil {
		return nil, fmt.Errorf("could not marshal nodes: %w", err)
	}

	resp, err := http.Post(serverURL+"/dag/nodes", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result pushResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("could not decode response: %w", err)
	}

	return &result, nil
}
