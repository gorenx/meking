package activation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/knowledge/mas"
)

const ModelFormula = "recall-stability-1"

// Model binds the archived parameters and their identity to the same scheduler.
// A caller cannot accidentally label one calculation with another model's ID.
type Model struct {
	config    mas.Config
	id        string
	scheduler *mas.Scheduler
}

func NewModel(config mas.Config) (Model, error) {
	scheduler, err := mas.NewScheduler(config)
	if err != nil {
		return Model{}, err
	}
	encoded, err := json.Marshal(struct {
		Formula    string
		Parameters [21]float64
	}{ModelFormula, config.Parameters})
	if err != nil {
		return Model{}, err
	}
	return Model{config: config, id: digest(encoded), scheduler: scheduler}, nil
}

func (model Model) ID() string         { return model.id }
func (model Model) Config() mas.Config { return model.config }

// RestoreModel validates the archived identity and calculation rules when a
// persistence adapter decodes a model. No storage operation is performed here.
func RestoreModel(id, formula string, parameters []float64) (Model, error) {
	if formula != ModelFormula || len(parameters) != 21 {
		return Model{}, fmt.Errorf("%w: invalid archived model", ErrDataIntegrity)
	}
	var config mas.Config
	copy(config.Parameters[:], parameters)
	model, err := NewModel(config)
	if err != nil || model.ID() != id {
		return Model{}, fmt.Errorf("%w: model content does not match identity", ErrDataIntegrity)
	}
	return model, nil
}

// Protocol is an immutable startup snapshot of the exact evaluation instructions
// and schemas offered to the external Agent. Its digest includes the language.
type Protocol struct {
	version      string
	language     string
	prompt       string
	inputSchema  string
	outputSchema string
	digest       string
}

func NewProtocol(version, language, prompt, inputSchema, outputSchema string) (Protocol, error) {
	if version != "1" || (language != "zh-CN" && language != "en") || !validText(prompt) {
		return Protocol{}, errors.New("activation: invalid evaluation protocol")
	}
	for _, schema := range []string{inputSchema, outputSchema} {
		var object map[string]any
		if err := json.Unmarshal([]byte(schema), &object); err != nil || object == nil {
			return Protocol{}, errors.New("activation: protocol schema must be a JSON object")
		}
	}
	encoded, err := json.Marshal([]string{version, language, prompt, inputSchema, outputSchema})
	if err != nil {
		return Protocol{}, err
	}
	return Protocol{version: version, language: language, prompt: prompt, inputSchema: inputSchema, outputSchema: outputSchema, digest: digest(encoded)}, nil
}

func (protocol Protocol) Version() string      { return protocol.version }
func (protocol Protocol) Language() string     { return protocol.language }
func (protocol Protocol) Prompt() string       { return protocol.prompt }
func (protocol Protocol) InputSchema() string  { return protocol.inputSchema }
func (protocol Protocol) OutputSchema() string { return protocol.outputSchema }
func (protocol Protocol) Digest() string       { return protocol.digest }

func RestoreProtocol(id, version, language, prompt, inputSchema, outputSchema string) (Protocol, error) {
	protocol, err := NewProtocol(version, language, prompt, inputSchema, outputSchema)
	if err != nil || protocol.Digest() != id {
		return Protocol{}, fmt.Errorf("%w: protocol content does not match identity", ErrDataIntegrity)
	}
	return protocol, nil
}

func digest(content []byte) string {
	value := sha256.Sum256(content)
	return hex.EncodeToString(value[:])
}
