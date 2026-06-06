package recognizer

import "testing"

func TestExtractText(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"final text", `{"text": "こんにちは"}`, "こんにちは"},
		{"partial text", `{"partial": "こん"}`, "こん"},
		{"text wins over partial", `{"text": "final", "partial": "partial"}`, "final"},
		{"empty object", `{}`, ""},
		{"empty fields", `{"text": "", "partial": ""}`, ""},
		{"invalid json", `not json`, ""},
		{"empty string", ``, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractText(tt.json); got != tt.want {
				t.Errorf("extractText(%q) = %q, want %q", tt.json, got, tt.want)
			}
		})
	}
}
