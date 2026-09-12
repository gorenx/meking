package project

import (
	"os"
	"strconv"
	"strings"
)

// modelCredentials is the process and Project environment observed exactly
// once while Project loads Configuration.
type modelCredentials struct {
	process map[string]string
	project map[string]string
}

func loadModelCredentials(projectRoot string) modelCredentials {
	return modelCredentials{
		process: processEnvironment(),
		project: readProjectEnvironment(projectRoot),
	}
}

func (credentials modelCredentials) resolveSettings(settings Settings) (Settings, error) {
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	resolved := settings
	resolved.CompletionModels = cloneModelConfigMap(settings.CompletionModels)
	resolved.EmbeddingModels = cloneModelConfigMap(settings.EmbeddingModels)
	resolveModelCredentials(credentials, &resolved)
	return resolved, nil
}

func resolveModelCredentials(credentials modelCredentials, settings *Settings) {
	resolve := func(models map[string]ModelConfig) {
		for id, model := range models {
			model.APIKey = credentials.resolve(model.APIKey)
			models[id] = model
		}
	}
	resolve(settings.CompletionModels)
	resolve(settings.EmbeddingModels)
}

func (credentials modelCredentials) resolve(configured string) string {
	configured = strings.TrimSpace(configured)
	if !strings.HasPrefix(configured, "${") || !strings.HasSuffix(configured, "}") {
		return configured
	}
	name := configured[2 : len(configured)-1]
	if value, found := credentials.process[name]; found && credentialValueConfigured(value) {
		return strings.TrimSpace(value)
	}
	if value := strings.TrimSpace(credentials.project[name]); credentialValueConfigured(value) {
		return value
	}
	return configured
}

func processEnvironment() map[string]string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	return values
}

func readProjectEnvironment(projectRoot string) map[string]string {
	content, err := readProjectText(projectRoot, ".env")
	if err != nil {
		return nil
	}
	return parseProjectEnvironment(content)
}

func parseProjectEnvironment(content string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			if decoded, decodeErr := strconv.Unquote(value); decodeErr == nil {
				value = decoded
			}
		} else if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}
		if key != "" {
			values[key] = value
		}
	}
	return values
}

func cloneModelConfigMap(source map[string]ModelConfig) map[string]ModelConfig {
	result := make(map[string]ModelConfig, len(source))
	for id, model := range source {
		result[id] = model
	}
	return result
}
