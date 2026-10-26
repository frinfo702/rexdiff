package diffview

import (
	"strings"
	"testing"
)

func TestScopeOf(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"python multiline def", "import os\n\ndef create(\n    a: int,\n) -> List:\n    x = 1\n    if a:\n        y = 2\n", "def create("},
		{"python method", "class A:\n    def m(self):\n        if x:\n            y = 2\n", "def m(self):"},
		{"go func", "package p\n\nfunc (v *View) text() string {\n\tif x {\n\t\ty()\n", "func (v *View) text() string {"},
		{"c function", "#include <x>\nint main(void) {\n    if (x) {\n        y();\n", "int main(void) {"},
		{"top level", "a = 1\nb = 2\nc = 3\n", ""},
	} {
		lines := strings.Split(tc.src, "\n")
		if got := scopeOf(lines, len(lines)-2); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
