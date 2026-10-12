// Package strictjson reads the JSON files people write by hand
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Unmarshal is json.Unmarshal with unknown fields as errors, since they are
// almost always a misspelled one, and the line and column on a syntax error
func Unmarshal(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return withPosition(data, err)
	}
	return nil
}

func withPosition(data []byte, err error) error {
	var syntax *json.SyntaxError
	var wrongType *json.UnmarshalTypeError
	offset := int64(-1)
	switch {
	case errors.As(err, &syntax):
		offset = syntax.Offset
	case errors.As(err, &wrongType):
		offset = wrongType.Offset
	}
	if offset < 0 {
		return err
	}
	// the offset counts the byte that went wrong
	before := data[:min(int(offset), len(data))]
	line := bytes.Count(before, []byte("\n")) + 1
	column := max(len(before)-bytes.LastIndexByte(before, '\n')-1, 1)
	return fmt.Errorf("line %d, column %d: %w", line, column, err)
}
