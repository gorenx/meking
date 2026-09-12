package mcp

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/memoria-space/meking/memory/activation"
)

//go:embed schemas/recall-submission.json
var recallSubmissionSchema string

//go:embed schemas/recall-observation.json
var recallObservationSchema string

//go:embed schemas/recall-protocol.json
var recallProtocolSchema string

// RecallProtocols binds the transport schemas to the exact startup prompt texts.
// Assembly archives these same values and supplies them to the application.
func RecallProtocols(chinese, english string) ([]activation.Protocol, error) {
	// Bind compact schema bytes, including field descriptions, to protocol
	// identities. Object keys stay sorted; whitespace is not part of identity.
	var input, output bytes.Buffer
	if err := json.Compact(&output, []byte(recallSubmissionSchema)); err != nil {
		return nil, fmt.Errorf("decode recall submission schema: %w", err)
	}
	if err := json.Compact(&input, []byte(recallObservationSchema)); err != nil {
		return nil, fmt.Errorf("decode recall observation schema: %w", err)
	}
	result := make([]activation.Protocol, 0, 2)
	for _, item := range []struct{ language, prompt string }{{"zh-CN", chinese}, {"en", english}} {
		protocol, err := activation.NewProtocol(
			"1",
			item.language,
			item.prompt,
			input.String(),
			output.String(),
		)
		if err != nil {
			return nil, err
		}
		result = append(result, protocol)
	}
	return result, nil
}
