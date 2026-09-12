package main

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/memoria-space/meking/cmd/meking/internal/httpservice"
	"github.com/spf13/cobra"
)

const (
	defaultStartAddress    = "127.0.0.1"
	defaultStartPort       = 8080
	defaultAnalysisCommand = "meking-analysis-sidecar"
)

type projectServerRunner func(
	context.Context,
	httpservice.Config,
	io.Writer,
	io.Writer,
) error

func newStartCommand() *cobra.Command {
	return newStartCommandWithRunner(httpservice.Run)
}

func newStartCommandWithRunner(run projectServerRunner) *cobra.Command {
	config := httpservice.Config{
		ProjectRoot:     ".",
		AnalysisCommand: defaultAnalysisCommand,
		Address:         defaultStartAddress,
		Port:            defaultStartPort,
	}

	command := &cobra.Command{
		Use:   "start",
		Short: "启动当前 Project 的嵌入式 Web 服务",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateProjectServerConfig(config); err != nil {
				return err
			}
			return run(cmd.Context(), config, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	flags := command.Flags()
	flags.StringVarP(&config.ProjectRoot, "root", "r", config.ProjectRoot, "Project 根目录")
	flags.StringVar(
		&config.AnalysisCommand,
		"analysis-command",
		config.AnalysisCommand,
		"Sentence 或富文档处理使用的本机 Analysis 可执行文件",
	)
	flags.StringVar(&config.Address, "address", config.Address, "HTTP 监听地址")
	flags.IntVar(&config.Port, "port", config.Port, "HTTP 监听端口")
	return command
}

func validateProjectServerConfig(config httpservice.Config) error {
	if strings.TrimSpace(config.ProjectRoot) == "" {
		return errors.New("start project root is required")
	}
	if strings.TrimSpace(config.Address) == "" {
		return errors.New("start address is required")
	}
	if config.Port <= 0 || config.Port > 65535 {
		return errors.New("start port must be between 1 and 65535")
	}
	return nil
}
