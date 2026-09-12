package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const analysisCacheFormatVersion = 2

// CacheKeyInput contains only analysis-result identity. InputHash must be the
// hash of content bytes; callers must not pass raw document text or paths.
type CacheKeyInput struct {
	Capability        Capability
	ContractVersion   int
	ConfigurationHash string
	InputHash         string
}

// CacheEntry stores one complete successful response. Failed, cancelled, or
// partial operations never satisfy this contract and must not be written.
type CacheEntry struct {
	Capability Capability
	Payload    []byte
	CreatedAt  time.Time
}

// Cache isolates deterministic NLP/conversion results from model usage cache.
type Cache interface {
	Get(ctx context.Context, key string) (CacheEntry, bool, error)
	Put(ctx context.Context, key string, entry CacheEntry) error
	Close() error
}

// CreateCacheKey produces a stable key without embedding private corpus data.
func CreateCacheKey(input CacheKeyInput) (string, error) {
	input.ConfigurationHash = strings.TrimSpace(input.ConfigurationHash)
	input.InputHash = strings.TrimSpace(input.InputHash)
	if !validCapability(input.Capability) {
		return "", errors.New("analysis cache capability is invalid")
	}
	if input.ContractVersion <= 0 {
		return "", errors.New("analysis cache contract version must be positive")
	}
	if input.ConfigurationHash == "" || input.InputHash == "" {
		return "", errors.New("analysis cache configuration and input hashes are required")
	}
	payload, err := json.Marshal(struct {
		FormatVersion     int        `json:"format_version"`
		Capability        Capability `json:"capability"`
		ContractVersion   int        `json:"contract_version"`
		ConfigurationHash string     `json:"configuration_hash"`
		InputHash         string     `json:"input_hash"`
	}{
		FormatVersion: analysisCacheFormatVersion, Capability: input.Capability,
		ContractVersion: input.ContractVersion, ConfigurationHash: input.ConfigurationHash,
		InputHash: input.InputHash,
	})
	if err != nil {
		return "", fmt.Errorf("marshal analysis cache key: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func cacheHash(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal analysis cache identity: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (c *Client) cacheKey(
	capability Capability,
	configurationHash string,
	inputHash string,
) (string, bool, error) {
	if c.cache == nil {
		return "", false, nil
	}
	key, err := CreateCacheKey(CacheKeyInput{
		Capability: capability, ContractVersion: ContractVersion,
		ConfigurationHash: configurationHash, InputHash: inputHash,
	})
	if err != nil {
		return "", false, err
	}
	return key, true, nil
}

func (c *Client) readCache(
	ctx context.Context,
	key string,
	capability Capability,
	target any,
) (bool, error) {
	entry, found, err := c.cache.Get(ctx, key)
	if err != nil || !found {
		return found, err
	}
	if entry.Capability != capability {
		return false, errors.New("analysis cache identity is invalid")
	}
	if err := decodeSingleJSON(entry.Payload, target); err != nil {
		return false, fmt.Errorf("decode analysis cache entry: %w", err)
	}
	return true, nil
}

func (c *Client) writeCache(
	ctx context.Context,
	key string,
	capability Capability,
	value any,
) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode analysis cache entry: %w", err)
	}
	return c.cache.Put(ctx, key, CacheEntry{
		Capability: capability, Payload: payload, CreatedAt: time.Now().UTC(),
	})
}
