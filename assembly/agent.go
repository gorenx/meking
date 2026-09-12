package assembly

import (
	"fmt"

	"github.com/memoria-space/meking/agent"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/project"
)

type agentClients struct {
	completion      *agent.OpenAICompletion
	queryCompletion *agent.OpenAICompletion
	embedding       *agent.OpenAIEmbedding
}

func openAgent(configuration project.Configuration) (agentClients, error) {
	completionConfiguration := configuration.Completion()
	completion, err := agent.NewOpenAICompletion(agent.OpenAICompletionConfig{
		APIKey:           completionConfiguration.APIKey,
		Model:            completionConfiguration.Model,
		BaseURL:          completionConfiguration.BaseURL,
		StructuredOutput: agent.StructuredOutputMode(completionConfiguration.StructuredOutput),
	})
	if err != nil {
		return agentClients{}, fmt.Errorf("create completion Agent client: %w", err)
	}
	queryConfiguration := configuration.QueryCompletion()
	queryCompletion, err := agent.NewOpenAICompletion(agent.OpenAICompletionConfig{
		APIKey:           queryConfiguration.APIKey,
		Model:            queryConfiguration.Model,
		BaseURL:          queryConfiguration.BaseURL,
		StructuredOutput: agent.StructuredOutputMode(queryConfiguration.StructuredOutput),
	})
	if err != nil {
		return agentClients{}, fmt.Errorf("create Query Agent client: %w", err)
	}
	embeddingConfiguration := configuration.Embedding()
	embedding, err := agent.NewOpenAIEmbedding(agent.OpenAIEmbeddingConfig{
		APIKey:  embeddingConfiguration.APIKey,
		Model:   embeddingConfiguration.Model,
		BaseURL: embeddingConfiguration.BaseURL,
	})
	if err != nil {
		return agentClients{}, fmt.Errorf("create embedding Agent client: %w", err)
	}
	return agentClients{
		completion:      completion,
		queryCompletion: queryCompletion,
		embedding:       embedding,
	}, nil
}

func corpusTokenizer(configuration project.Configuration) (*textunits.TiktokenTokenizer, error) {
	tokens, err := textunits.NewTiktokenTokenizer(configuration.Chunking.EncodingModel)
	if err != nil {
		return nil, fmt.Errorf("create Corpus tokenizer: %w", err)
	}
	return tokens, nil
}
