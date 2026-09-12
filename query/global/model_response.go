package global

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func decodeJSONObject(response string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(response), &object); err != nil || object == nil {
		if err == nil {
			err = errors.New("decoded value is not an object")
		}
		return nil, fmt.Errorf("decode Global model response as JSON object: %w", err)
	}
	return object, nil
}

func parseInteger(data json.RawMessage) (int, error) {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		value, parseErr := strconv.Atoi(strings.TrimSpace(text))
		if parseErr != nil {
			return 0, fmt.Errorf("Global model value is not an integer: %w", parseErr)
		}
		return value, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return 0, fmt.Errorf("decode Global model number: %w", err)
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, errors.New("Global model value is not numeric")
	}
	integer, err := number.Int64()
	if err == nil {
		return int(integer), nil
	}
	floating, err := number.Float64()
	if err != nil {
		return 0, fmt.Errorf("decode Global model number: %w", err)
	}
	return int(floating), nil
}
