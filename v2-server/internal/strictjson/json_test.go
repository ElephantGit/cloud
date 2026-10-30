package strictjson

import (
	"fmt"
	"testing"
)

func TestValid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  bool
	}{
		{"object", `{"text":"中文🙂","values":[null,true,1.25]}`, true},
		{"replacement_rune", `"�"`, true},
		{"replacement_escape", `"\ufffd"`, true},
		{"paired_surrogates", `"\ud83d\ude42"`, true},
		{"uppercase_pair", `"\uD83D\uDE42"`, true},
		{"minimum_pair", `"\ud800\udc00"`, true},
		{"maximum_pair", `"\udbff\udfff"`, true},
		{"literal_high_surrogate", `"\\ud800"`, true},
		{"literal_low_surrogate", `"\\udfff"`, true},
		{"literal_backslash_before_pair", `"\\\ud83d\ude42"`, true},
		{"escaped_quote", `"\"\\ud800"`, true},
		{"high_surrogate", `"\ud800"`, false},
		{"low_surrogate", `"\udc00"`, false},
		{"reversed_pair", `"\udc00\ud800"`, false},
		{"two_high_surrogates", `"\ud800\ud800"`, false},
		{"high_then_normal_escape", `"\ud800\u0061"`, false},
		{"high_then_text", `"\ud800x"`, false},
		{"high_then_escaped_backslash", `"\ud800\\udc00"`, false},
		{"high_then_escaped_quote", `"\ud800\""`, false},
		{"high_in_key", `{"\ud800":"value"}`, false},
		{"high_in_nested_value", `{"values":["\ud800"]}`, false},
		{"raw_invalid_utf8", "\"\xff\"", false},
		{"raw_surrogate_utf8", "\"\xed\xa0\x80\"", false},
		{"invalid_syntax", `{"text":}`, false},
		{"truncated_escape", `"\u12`, false},
		{"invalid_hex", `"\ud80z"`, false},
		{"multiple_values", `{} {}`, false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Valid([]byte(tc.input)); got != tc.want {
				t.Fatalf("Valid(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestEverySingleUnicodeEscape(t *testing.T) {
	for code := 0; code <= 0xffff; code++ {
		input := []byte(fmt.Sprintf(`"\u%04x"`, code))
		want := code < 0xd800 || code > 0xdfff
		if got := Valid(input); got != want {
			t.Fatalf("Valid(%s) = %v, want %v", input, got, want)
		}
	}
}
