package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// TestReadmeSettings checks that rexdiff accepts the settings sample of
// the README. The test turns the Lua table into the JSON that rexdiff.lua
// passes: the table without key and placement.
func TestReadmeSettings(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, sample, ok := strings.Cut(string(b), "```lua\nlocal settings = ")
	if !ok {
		t.Fatal("the README has no lua block with local settings")
	}
	sample, _, _ = strings.Cut(sample, "```")
	p := &luaParser{src: sample}
	v, err := p.value()
	if err != nil {
		t.Fatalf("the settings of the README: %v", err)
	}
	opts, ok := v.(map[string]any)
	if !ok || len(opts) < 3 {
		t.Fatalf("the settings of the README = %v, want a table of settings", v)
	}
	delete(opts, "key")
	delete(opts, "placement")
	text, err := json.Marshal(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckConfig(string(text)); err != nil {
		t.Errorf("CheckConfig(%s) = %v, want nil", text, err)
	}
}

// TestLuaParser checks the parser of the Lua tables of the README.
func TestLuaParser(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, src, want string }{
		{
			"table", `{ a = "x", -- note
		    b = false, c = 2, d = { e = { "S", "T" } } }`,
			`{"a":"x","b":false,"c":2,"d":{"e":["S","T"]}}`,
		},
		{"escape", `"a\"b"`, `"a\"b"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := &luaParser{src: tt.src}
			v, err := p.value()
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := json.Marshal(v); string(got) != tt.want {
				t.Errorf("value = %s, want %s", got, tt.want)
			}
		})
	}
	for _, bad := range []string{`{ a = }`, `{ a = "x"`, `"x`, `nil`} {
		p := &luaParser{src: bad}
		if v, err := p.value(); err == nil {
			t.Errorf("value of %q = %v, want an error", bad, v)
		}
	}
}

// luaParser reads a Lua table of strings, booleans, numbers, and tables.
// That is the form of the settings in the README.
type luaParser struct {
	src string
	pos int
}

// skip moves past the spaces and the comments.
func (p *luaParser) skip() {
	for p.pos < len(p.src) {
		switch {
		case unicode.IsSpace(rune(p.src[p.pos])):
			p.pos++
		case strings.HasPrefix(p.src[p.pos:], "--"):
			end := strings.IndexByte(p.src[p.pos:], '\n')
			if end < 0 {
				end = len(p.src) - p.pos
			}
			p.pos += end
		default:
			return
		}
	}
}

// peek returns the next byte after the spaces, or 0 at the end.
func (p *luaParser) peek() byte {
	p.skip()
	if p.pos == len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

// word returns the name or the number at the position.
func (p *luaParser) word() string {
	start := p.pos
	for p.pos < len(p.src) {
		c := rune(p.src[p.pos])
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '.' {
			break
		}
		p.pos++
	}
	return p.src[start:p.pos]
}

// value reads a value.
func (p *luaParser) value() (any, error) {
	switch p.peek() {
	case '{':
		p.pos++
		return p.table()
	case '"':
		end := p.pos + 1
		for end < len(p.src) && p.src[end] != '"' {
			if p.src[end] == '\\' {
				end++
			}
			end++
		}
		if end >= len(p.src) {
			return nil, fmt.Errorf("no end of the string at %d", p.pos)
		}
		s, err := strconv.Unquote(p.src[p.pos : end+1])
		p.pos = end + 1
		return s, err
	}
	w := p.word()
	switch w {
	case "true", "false":
		return w == "true", nil
	}
	if n, err := strconv.ParseFloat(w, 64); err == nil {
		return n, nil
	}
	return nil, fmt.Errorf("no value at %d: %q", p.pos, w)
}

// table reads the fields of a table after its "{". A table with names is
// a map. A table without names is a list.
func (p *luaParser) table() (any, error) {
	fields := map[string]any{}
	var list []any
	for {
		if p.peek() == '}' {
			p.pos++
			break
		}
		start := p.pos
		name := p.word()
		var v any
		var err error
		if name != "" && p.peek() == '=' {
			p.pos++
			if v, err = p.value(); err != nil {
				return nil, err
			}
			fields[name] = v
		} else {
			p.pos = start
			if v, err = p.value(); err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		switch p.peek() {
		case ',':
			p.pos++
		case '}':
		default:
			return nil, fmt.Errorf("no , or } at %d", p.pos)
		}
	}
	if len(list) > 0 {
		return list, nil
	}
	return fields, nil
}
