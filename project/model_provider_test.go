package project

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type firstReleaseProvider struct {
	t                 *testing.T
	chatRequests      atomic.Int32
	embeddingRequests atomic.Int32
}

func (p *firstReleaseProvider) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	p.t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	switch request.URL.Path {
	case "/v1/chat/completions":
		p.serveChatCompletion(writer, request)
	case "/v1/embeddings":
		p.serveEmbedding(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

func (p *firstReleaseProvider) serveChatCompletion(writer http.ResponseWriter, request *http.Request) {
	p.chatRequests.Add(1)
	var body struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		ResponseFormat struct {
			Type       string `json:"type"`
			JSONSchema struct {
				Name string `json:"name"`
			} `json:"json_schema"`
		} `json:"response_format"`
		Stream bool `json:"stream"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		p.t.Errorf("decode chat request: %v", err)
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	if body.Stream {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(writer, "data: {\"id\":\"chatcmpl-stream\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"completion-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ready\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"id\":\"chatcmpl-stream\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"completion-test\",\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":10,\"total_tokens\":30}}\n\n")
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
		return
	}
	content := ""
	switch body.ResponseFormat.Type {
	case "json_schema":
		if body.ResponseFormat.JSONSchema.Name == "knowledge_graph" {
			content = `{"entities":[{"name":"ATLAS","type":"ORGANIZATION","aliases":[],"description":"Atlas coordinates research"},{"name":"BEACON","type":"ORGANIZATION","aliases":[],"description":"Beacon supports Atlas"}],"relations":[{"source":"ATLAS","target":"BEACON","type":"COLLABORATION","description":"Atlas collaborates with Beacon","weight":1}]}`
		} else {
			content = `{"title":"Atlas network","summary":"Atlas and Beacon collaborate.","findings":[{"summary":"Collaboration","explanation":"Atlas works with Beacon [Data: Entities (0, 1); Relationships (0)]."}],"rating":8,"rating_explanation":"The relationship is explicit."}`
		}
	case "json_object":
		content = `{"points":[{"description":"Atlas and Beacon collaborate [Data: Reports (0)]","score":90}]}`
	default:
		if len(body.Messages) == 1 {
			content = `("entity"<|>"ATLAS"<|>"ORGANIZATION"<|>"Atlas coordinates research")##("entity"<|>"BEACON"<|>"ORGANIZATION"<|>"Beacon supports Atlas")##("relationship"<|>"ATLAS"<|>"BEACON"<|>"COLLABORATION"<|>"Atlas collaborates with Beacon"<|>1)##<|COMPLETE|>`
		} else if strings.Contains(body.Messages[0].Content, "----Analyst") {
			content = "Atlas and Beacon collaborate [Data: Reports (0)]."
		} else {
			content = "Atlas works with Beacon [Data: Sources (0)]."
		}
	}
	p.writeJSON(writer, map[string]any{
		"id": "chatcmpl-first-release", "object": "chat.completion", "created": 1,
		"model": "completion-test",
		"choices": []map[string]any{{
			"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": content},
		}},
		"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30},
	})
}

func (p *firstReleaseProvider) serveEmbedding(writer http.ResponseWriter, request *http.Request) {
	p.embeddingRequests.Add(1)
	var body struct {
		Input []string `json:"input"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		p.t.Errorf("decode embedding request: %v", err)
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	data := make([]map[string]any, len(body.Input))
	for index := range body.Input {
		data[index] = map[string]any{
			"object": "embedding", "index": index, "embedding": []float64{1, 0},
		}
	}
	p.writeJSON(writer, map[string]any{
		"object": "list", "model": "embedding-test", "data": data,
		"usage": map[string]any{"prompt_tokens": len(body.Input), "total_tokens": len(body.Input)},
	})
}

func (p *firstReleaseProvider) writeJSON(writer http.ResponseWriter, value any) {
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		p.t.Errorf("encode provider response: %v", err)
	}
}
