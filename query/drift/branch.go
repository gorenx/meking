package drift

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type branchResponse struct {
	answer    string
	score     int
	followUps []string
}

func parseBranchResponse(response string, followUpLimit int) (branchResponse, error) {
	var wire struct {
		Answer          string   `json:"response"`
		Score           *int     `json:"score"`
		FollowUpQueries []string `json:"follow_up_queries"`
	}
	decoder := json.NewDecoder(strings.NewReader(response))
	if err := decoder.Decode(&wire); err != nil {
		return branchResponse{}, fmt.Errorf("decode DRIFT branch response: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return branchResponse{}, errors.New("DRIFT branch response contains trailing JSON")
		}
		return branchResponse{}, fmt.Errorf("decode DRIFT branch response trailer: %w", err)
	}
	if strings.TrimSpace(wire.Answer) == "" {
		return branchResponse{}, errors.New("DRIFT branch response has no answer")
	}
	if wire.Score == nil || *wire.Score < 0 || *wire.Score > 100 {
		return branchResponse{}, errors.New("DRIFT branch response score must be between 0 and 100")
	}
	if len(wire.FollowUpQueries) > followUpLimit {
		return branchResponse{}, fmt.Errorf(
			"DRIFT branch response contains %d follow-up queries, limit is %d",
			len(wire.FollowUpQueries),
			followUpLimit,
		)
	}
	followUps := make([]string, len(wire.FollowUpQueries))
	for index, question := range wire.FollowUpQueries {
		question = strings.TrimSpace(question)
		if question == "" {
			return branchResponse{}, fmt.Errorf("DRIFT branch follow-up query %d is empty", index)
		}
		followUps[index] = question
	}
	return branchResponse{answer: wire.Answer, score: *wire.Score, followUps: followUps}, nil
}
