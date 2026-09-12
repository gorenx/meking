package analysis

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	// ContractVersion is the only HTTP/JSON protocol understood by this build.
	ContractVersion = 2
	// APIVersion scopes endpoint paths independently from payload evolution.
	APIVersion = "v1"
)

// Capability identifies one optional analysis operation offered by a healthy
// local sidecar. It describes transport availability, not a corpus or graph
// domain object.
type Capability string

const (
	CapabilitySentences   Capability = "sentences"
	CapabilityNounPhrases Capability = "noun_phrases"
	CapabilityMarkItDown  Capability = "markitdown"
)

// Health is the authenticated startup contract used before a caller may depend
// on the sidecar.
type Health struct {
	ContractVersion int          `json:"contract_version"`
	APIVersion      string       `json:"api_version"`
	Capabilities    []Capability `json:"capabilities"`
	Resources       []string     `json:"resources"`
}

func (h Health) validate(required []Capability) error {
	if h.ContractVersion != ContractVersion {
		return fmt.Errorf(
			"analysis contract version %d does not match required version %d",
			h.ContractVersion, ContractVersion,
		)
	}
	if strings.TrimSpace(h.APIVersion) != APIVersion {
		return fmt.Errorf("analysis API version %q does not match required version %q", h.APIVersion, APIVersion)
	}
	seen := make(map[Capability]struct{}, len(h.Capabilities))
	for _, capability := range h.Capabilities {
		if !validCapability(capability) {
			return fmt.Errorf("analysis returned unknown capability %q", capability)
		}
		if _, ok := seen[capability]; ok {
			return fmt.Errorf("analysis returned duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
	}
	for _, capability := range required {
		if !validCapability(capability) {
			return fmt.Errorf("unknown required analysis capability %q", capability)
		}
		if _, ok := seen[capability]; !ok {
			return fmt.Errorf("analysis capability %q is not installed", capability)
		}
	}
	if !slices.IsSorted(h.Resources) {
		return errors.New("analysis resources must use stable sorted order")
	}
	for index, resource := range h.Resources {
		if strings.TrimSpace(resource) == "" {
			return errors.New("analysis resource names must be non-empty")
		}
		if index > 0 && resource == h.Resources[index-1] {
			return fmt.Errorf("analysis returned duplicate resource %q", resource)
		}
	}
	return nil
}

func validCapability(capability Capability) bool {
	switch capability {
	case CapabilitySentences, CapabilityNounPhrases, CapabilityMarkItDown:
		return true
	default:
		return false
	}
}

type errorEnvelope struct {
	Error remoteError `json:"error"`
}

type remoteError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type readyMessage struct {
	Event           string       `json:"event"`
	ContractVersion int          `json:"contract_version"`
	Port            int          `json:"port"`
	PID             int          `json:"pid"`
	Capabilities    []Capability `json:"capabilities"`
}
