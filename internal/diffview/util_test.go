package diffview

import "testing"

func TestCutLeft(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"日本x", 0, "日本x"},
		{"日本x", 1, " 本x"},
		{"日本x", 2, "本x"},
		{"日本x", 3, " x"},
		{"日本x", 9, ""},
		{"\x1b[31m日本\x1b[m", 1, " \x1b[31m本\x1b[m"},
	}
	for _, tt := range tests {
		if got := CutLeft(tt.s, tt.n); got != tt.want {
			t.Errorf("CutLeft(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}
