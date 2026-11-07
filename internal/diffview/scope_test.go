package diffview

import (
	"strings"
	"testing"
)

func TestScopesOf(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"python multiline def", "import os\n\ndef create(\n    a: int,\n) -> List:\n    x = 1\n    if a:\n        y = 2\n", "def create("},
		{"python method", "class A:\n    def m(self):\n        if x:\n            y = 2\n", "def m(self):"},
		{"go func", "package p\n\nfunc (v *View) text() string {\n\tif x {\n\t\ty()\n", "func (v *View) text() string {"},
		{"c function", "#include <x>\nint main(void) {\n    if (x) {\n        y();\n", "int main(void) {"},
		{"top level", "a = 1\nb = 2\nc = 3\n", ""},
		{"sibling closed", "def a():\n    x = 1\ndef b():\n    y = 2\n", "def b():"},
		{"deeper sibling", "class A:\n    def m(self):\n        x = 1\n    y = 2\n", "class A:"},
	} {
		lines := strings.Split(tc.src, "\n")
		if got := scopesOf(lines, []int{len(lines) - 2})[0]; got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestScopesOfMany checks several hunks in one pass.
func TestScopesOfMany(t *testing.T) {
	lines := strings.Split("def a():\n    x = 1\n\ndef b():\n    y = 2\nz = 3\n", "\n")
	got := scopesOf(lines, []int{0, 1, 4, 5})
	want := []string{"", "def a():", "def b():", ""}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}
