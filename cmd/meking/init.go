package main

import (
	"errors"
	"fmt"

	"github.com/memoria-space/meking/project"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	var options project.InitializeProject
	options.Root = "."
	options.CompletionModel = project.DefaultCompletionModel
	options.EmbeddingModel = project.DefaultEmbeddingModel

	command := &cobra.Command{
		Use:   "init [project-path]",
		Short: "初始化本地 Meking Project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if cmd.Flags().Changed("root") {
					return errors.New("project path and --root cannot be used together")
				}
				options.Root = args[0]
			}
			result, err := project.NewLocalProjectService().Initialize(cmd.Context(), options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"已初始化 Meking Project：%s（创建 %d，覆盖 %d，保留 %d）\n",
				result.Root, len(result.Created), len(result.Overwritten), len(result.Preserved),
			)
			return err
		},
	}

	flags := command.Flags()
	flags.StringVarP(&options.Root, "root", "r", options.Root, "Project 根目录")
	flags.StringVarP(&options.CompletionModel, "model", "m", options.CompletionModel, "OpenAI Completion 模型")
	flags.StringVarP(&options.EmbeddingModel, "embedding", "e", options.EmbeddingModel, "OpenAI Embedding 模型")
	flags.StringVar(&options.CompletionBaseURL, "model-base-url", "", "OpenAI Completion API 根地址")
	flags.StringVar(&options.EmbeddingBaseURL, "embedding-base-url", "", "OpenAI Embedding API 根地址")
	flags.BoolVarP(&options.Force, "force", "f", false, "覆盖现有配置、.env 和 Prompt")
	return command
}
