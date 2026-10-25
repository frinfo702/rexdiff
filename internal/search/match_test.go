package search

import "testing"

// TestMatch checks the smart case of herdr-reviewr.
func TestMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		text, query string
		want        bool
	}{
		{"Foo bar", "foo", true},
		{"foo bar", "Foo", false},
		{"Foo bar", "Foo", true},
		{"FOO", "foo", true},
		{"x := 1", "", false},
		{"", "a", false},
		{"ÉCOLE", "école", false}, // only ASCII ignores case
		{"école", "école", true},
		{"Éa", "Éa", true},
		{"\tx", "\tx", true},
		{"ab", "abc", false},
	}
	for _, tt := range tests {
		if got := Match(tt.text, tt.query); got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.text, tt.query, got, tt.want)
		}
	}
}
