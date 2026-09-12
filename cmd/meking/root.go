package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:               "meking",
		Short:             "基于知识图谱的本地检索增强生成工具",
		SilenceErrors:     true,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return resolveProjectRootFlag(cmd)
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(
		newInitCommand(),
		newMCPCommand(),
		newStartCommand(),
	)
	return root
}

// resolveProjectRootFlag freezes a command's Project location before the
// use case starts. Relative paths are interpreted against the process working
// directory so all later settings and artifact access uses one absolute root.
func resolveProjectRootFlag(command *cobra.Command) error {
	flag := command.Flags().Lookup("root")
	if flag == nil {
		return nil
	}
	configured := strings.TrimSpace(flag.Value.String())
	if configured == "" {
		return nil
	}
	absolute, err := filepath.Abs(configured)
	if err != nil {
		return fmt.Errorf("resolve Project root: %w", err)
	}
	return flag.Value.Set(filepath.Clean(absolute))
}
