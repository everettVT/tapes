package mergecmder

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/papercomputeco/tapes/cmd/tapes/sqlitepath"
	"github.com/papercomputeco/tapes/pkg/storage/sqlite"
)

const mergeLongDesc string = `Merge one or more source SQLite databases into a target.

Content-addressing makes this a simple union: nodes that already
exist in the target are skipped (deduped by hash).

Examples:
  tapes merge source1.sqlite source2.sqlite
  tapes merge --sqlite /tmp/merged.sqlite ~/alice/tapes.db ~/bob/tapes.db`

const mergeShortDesc string = "Merge SQLite databases"

type mergeCommander struct {
	sqlitePath string
}

func NewMergeCmd() *cobra.Command {
	cmder := &mergeCommander{}

	cmd := &cobra.Command{
		Use:   "merge [sources...]",
		Short: mergeShortDesc,
		Long:  mergeLongDesc,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmder.run(cmd.Context(), cmd, args)
		},
	}

	cmd.Flags().StringVarP(&cmder.sqlitePath, "sqlite", "s", "", "Path to target SQLite database")

	return cmd
}

func (c *mergeCommander) run(ctx context.Context, cmd *cobra.Command, sources []string) error {
	targetPath, err := sqlitepath.ResolveSQLitePath(c.sqlitePath)
	if err != nil {
		return fmt.Errorf("could not resolve target database: %w", err)
	}

	target, err := sqlite.NewDriver(ctx, targetPath)
	if err != nil {
		return fmt.Errorf("could not open target database %s: %w", targetPath, err)
	}
	defer target.Close()

	var totalNew, totalDuped int

	for _, srcPath := range sources {
		source, err := sqlite.NewDriver(ctx, srcPath)
		if err != nil {
			return fmt.Errorf("could not open source database %s: %w", srcPath, err)
		}

		nodes, err := source.List(ctx)
		if err != nil {
			source.Close()
			return fmt.Errorf("could not list nodes from %s: %w", srcPath, err)
		}

		var srcNew, srcDuped int
		for _, n := range nodes {
			isNew, err := target.Put(ctx, n)
			if err != nil {
				source.Close()
				return fmt.Errorf("could not put node %s: %w", n.Hash, err)
			}
			if isNew {
				srcNew++
			} else {
				srcDuped++
			}
		}

		totalNew += srcNew
		totalDuped += srcDuped
		source.Close()

		fmt.Fprintf(cmd.OutOrStdout(), "  %s: %d new, %d already existed\n", srcPath, srcNew, srcDuped)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Merged %d new nodes from %d sources (%d already existed) into %s\n",
		totalNew, len(sources), totalDuped, targetPath)

	return nil
}
