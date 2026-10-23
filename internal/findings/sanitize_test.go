package findings

import "testing"

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		// glamour v2.0.1 passes this OSC 52 clipboard write through unchanged.
		{"osc 52 clipboard write", "a\x1b]52;c;ZXZpbA==\x07b", "a]52;c;ZXZpbA==b"},
		{"csi clear screen", "\x1b[2Jtext", "[2Jtext"},
		{"8-bit csi", "\u009b31mred", "31mred"},
		{"other c1 control", "n\u0085el", "nel"},
		{"del", "x\x7fy", "xy"},
		{"nul", "n\x00ul", "nul"},
		{"carriage return", "c\rr", "cr"},
		{"invalid utf-8", "in\xffv\xc3", "inv"},
		{"tab and newline kept", "a\tb\nc", "a\tb\nc"},
		{"non-ascii kept", "ä 漢字 🙂", "ä 漢字 🙂"},
		// U+2028 LINE SEPARATOR is not a control character.
		{"line separator kept", "l\xe2\x80\xa8s", "l\xe2\x80\xa8s"},
		{"empty", "", ""},
		// Bidi controls reorder visible text in bidi-capable terminals. They are
		// written as UTF-8 bytes so this file holds no raw bidi character:
		// U+202E RLO, U+202C PDF.
		{"bidi override dropped", "r\xe2\x80\xaeto\xe2\x80\xacl", "rtol"},
		// U+2066 LRI, U+2067 RLI, U+2068 FSI, U+2069 PDI.
		{"bidi isolates dropped", "i\xe2\x81\xa6s\xe2\x81\xa7o\xe2\x81\xa8l\xe2\x81\xa9", "isol"},
		// U+200E LRM, U+200F RLM, U+061C ALM.
		{"directional marks dropped", "m\xe2\x80\x8ea\xe2\x80\x8fr\xd8\x9ck", "mark"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Sanitize(tt.in); got != tt.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
