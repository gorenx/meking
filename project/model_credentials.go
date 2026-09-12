package project

import (
	"errors"
	"regexp"
	"strings"
)

var modelCredentialEnvironmentPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

func validateModelCredentialReference(reference string) error {
	reference = strings.TrimSpace(reference)
	if !strings.HasPrefix(reference, "${") || !strings.HasSuffix(reference, "}") {
		return errors.New("model credential must be an environment reference")
	}
	environment := reference[2 : len(reference)-1]
	if !modelCredentialEnvironmentPattern.MatchString(environment) {
		return errors.New("model credential environment must be an uppercase environment variable name")
	}
	return nil
}

func credentialValueConfigured(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "<API_KEY>"
}
