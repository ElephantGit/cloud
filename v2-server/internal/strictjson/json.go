// Package strictjson validates JSON without accepting lossy Unicode replacement.
package strictjson

import (
	"encoding/json"
	"unicode/utf8"
)

// Valid reports whether data is valid UTF-8 JSON whose Unicode escapes contain
// no unpaired UTF-16 surrogates. encoding/json otherwise replaces those escapes
// with U+FFFD during unmarshaling, which can conceal damaged source data.
func Valid(data []byte) bool {
	if !utf8.Valid(data) || !json.Valid(data) {
		return false
	}
	// JSON syntax validation ensures every backslash begins a string escape and
	// every Unicode escape has four hexadecimal digits. Skip other escapes so
	// an escaped literal backslash cannot be mistaken for a Unicode escape.
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		i++
		if data[i] != 'u' {
			continue
		}
		value := hex4(data[i+1 : i+5])
		i += 4
		switch {
		case value >= 0xd800 && value <= 0xdbff:
			if len(data)-i <= 6 || data[i+1] != '\\' || data[i+2] != 'u' {
				return false
			}
			low := hex4(data[i+3 : i+7])
			if low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		case value >= 0xdc00 && value <= 0xdfff:
			return false
		}
	}
	return true
}

// hex4 decodes the four digits of a syntactically valid JSON Unicode escape.
func hex4(digits []byte) uint16 {
	var value uint16
	for _, digit := range digits {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value |= uint16(digit - '0')
		case digit >= 'A' && digit <= 'F':
			value |= uint16(digit-'A') + 10
		default:
			value |= uint16(digit-'a') + 10
		}
	}
	return value
}
