package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/memoria-space/meking/assembly"
	"github.com/spf13/cobra"
)

type mcpConfig struct {
	ProjectRoot     string
	AnalysisCommand string
}

type mcpRunner func(
	context.Context,
	mcpConfig,
	io.Reader,
	io.Writer,
	io.Writer,
) error

func newMCPCommand() *cobra.Command {
	return newMCPCommandWithRunner(runMCP)
}

func newMCPCommandWithRunner(run mcpRunner) *cobra.Command {
	configuration := mcpConfig{
		ProjectRoot:     ".",
		AnalysisCommand: defaultAnalysisCommand,
	}
	command := &cobra.Command{
		Use:   "mcp",
		Short: "通过 STDIO 提供 Agent Memory MCP Server",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validateMCPConfig(configuration); err != nil {
				return err
			}
			return run(
				command.Context(),
				configuration,
				command.InOrStdin(),
				command.OutOrStdout(),
				command.ErrOrStderr(),
			)
		},
	}
	flags := command.Flags()
	flags.StringVarP(&configuration.ProjectRoot, "root", "r", configuration.ProjectRoot, "Project 根目录")
	flags.StringVar(
		&configuration.AnalysisCommand,
		"analysis-command",
		configuration.AnalysisCommand,
		"Sentence 或富文档处理使用的本机 Analysis 可执行文件",
	)
	return command
}

func validateMCPConfig(configuration mcpConfig) error {
	if strings.TrimSpace(configuration.ProjectRoot) == "" {
		return errors.New("MCP Project root is required")
	}
	return nil
}

func runMCP(
	ctx context.Context,
	configuration mcpConfig,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
) (resultErr error) {
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	application, err := assembly.Open(ctx, assembly.Config{
		Root:            configuration.ProjectRoot,
		AnalysisCommand: configuration.AnalysisCommand,
		Logger:          logger,
	})
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, application.Close())
	}()
	toolset, err := application.MCP()
	if err != nil {
		return err
	}
	stdio := mcpserver.NewStdioServer(toolset.Server())
	return stdio.Listen(ctx, stdin, stdout)
}
