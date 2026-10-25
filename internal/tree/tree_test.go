package tree

import (
	"fmt"
	"slices"
	"testing"
)

// The tests come from file_list.rs of herdr-reviewr.

// changed returns changed entries for paths.
func changed(paths ...string) []Entry {
	var es []Entry
	for _, p := range paths {
		es = append(es, Entry{Path: p, Changed: true})
	}
	return es
}

// shape returns the rows as "depth:dir:name" or "depth:file:name".
func shape(rows []Row) []string {
	var out []string
	for _, r := range rows {
		kind := "file"
		if r.File < 0 {
			kind = "dir"
		}
		out = append(out, fmt.Sprintf("%d:%s:%s", r.Depth, kind, r.Name))
	}
	return out
}

// set returns a set of paths.
func set(paths ...string) map[string]bool {
	m := map[string]bool{}
	for _, p := range paths {
		m[p] = true
	}
	return m
}

func TestBuildShape(t *testing.T) {
	tests := []struct {
		name     string
		entries  []Entry
		toggled  map[string]bool
		expanded bool
		want     []string
	}{
		{
			name:     "directories before files",
			entries:  changed("src/app.rs", "src/ui.rs", "Cargo.toml"),
			expanded: true,
			want:     []string{"0:dir:src", "1:file:app.rs", "1:file:ui.rs", "0:file:Cargo.toml"},
		},
		{
			name:     "a chain of one child ends in a file row",
			entries:  changed("docs/plans/2026/plan.md"),
			expanded: true,
			want:     []string{"0:file:docs/plans/2026/plan.md"},
		},
		{
			name:     "a chain stops at a branch",
			entries:  changed("a/b/c/one.rs", "a/b/c/two.rs"),
			expanded: true,
			want:     []string{"0:dir:a/b/c", "1:file:one.rs", "1:file:two.rs"},
		},
		{
			name:     "a collapsed directory hides its children",
			entries:  changed("src/app.rs", "src/ui.rs"),
			toggled:  set("src"),
			expanded: true,
			want:     []string{"0:dir:src"},
		},
		{
			name:    "all files starts collapsed",
			entries: changed("src/app.rs", "src/ui.rs"),
			want:    []string{"0:dir:src"},
		},
		{
			name:    "a toggle expands in all files",
			entries: changed("src/app.rs", "src/ui.rs"),
			toggled: set("src"),
			want:    []string{"0:dir:src", "1:file:app.rs", "1:file:ui.rs"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shape(Build(tt.entries, tt.toggled, tt.expanded))
			if !slices.Equal(got, tt.want) {
				t.Errorf("Build = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildFileIndex(t *testing.T) {
	rows := Build(changed("z.rs", "a.rs"), nil, true)
	// The rows are in alphabetical order, so a.rs is first.
	if rows[0].File != 1 || rows[1].File != 0 {
		t.Errorf("file indices %d, %d; want 1, 0", rows[0].File, rows[1].File)
	}
	if rows[0].Dir != "" {
		t.Errorf("a file row has the directory %q, want none", rows[0].Dir)
	}
}

func TestBuildChangeMark(t *testing.T) {
	es := append(changed("src/ui.rs"),
		Entry{Path: "src/app.rs"}, Entry{Path: "docs/a.md"}, Entry{Path: "docs/b.md"})
	rows := Build(es, nil, false)
	if got, want := shape(rows), []string{"0:dir:docs", "0:dir:src"}; !slices.Equal(got, want) {
		t.Fatalf("Build = %q, want %q", got, want)
	}
	if rows[0].Changed || !rows[1].Changed {
		t.Errorf("changed docs %v, src %v; want false, true", rows[0].Changed, rows[1].Changed)
	}
	if rows[1].Dir != "src" || rows[1].File != -1 || rows[1].Expanded {
		t.Errorf("src row %+v, want a collapsed directory", rows[1])
	}

	// The mark stays when the directory expands.
	rows = Build(es, set("src"), false)
	want := []string{"0:dir:docs", "0:dir:src", "1:file:app.rs", "1:file:ui.rs"}
	if got := shape(rows); !slices.Equal(got, want) {
		t.Fatalf("Build = %q, want %q", got, want)
	}
	if !rows[1].Changed || !rows[1].Expanded {
		t.Errorf("expanded src: changed %v, expanded %v; want true, true",
			rows[1].Changed, rows[1].Expanded)
	}

	// A change deep in the tree marks each directory above it.
	es = append(changed("a/b/deep.rs"), Entry{Path: "a/top.rs"}, Entry{Path: "a/b/other.rs"})
	rows = Build(es, set("a"), false)
	want = []string{"0:dir:a", "1:dir:b", "1:file:top.rs"}
	if got := shape(rows); !slices.Equal(got, want) {
		t.Fatalf("Build = %q, want %q", got, want)
	}
	if !rows[0].Changed || !rows[1].Changed {
		t.Errorf("changed a %v, a/b %v; want true, true", rows[0].Changed, rows[1].Changed)
	}
	if rows[1].Dir != "a/b" {
		t.Errorf("the path of b is %q, want a/b", rows[1].Dir)
	}
}

func TestBuildIgnored(t *testing.T) {
	rows := Build([]Entry{{Path: "target", Ignored: true, Dir: true}}, nil, false)
	if len(rows) != 1 {
		t.Fatalf("Build = %q, want one row", shape(rows))
	}
	if r := rows[0]; !r.Ignored || r.File != -1 || r.Expanded || r.Changed {
		t.Errorf("ignored directory %+v, want an ignored, collapsed directory", r)
	}

	// The entries of an expanded ignored directory show under it.
	es := []Entry{
		{Path: "target", Ignored: true, Dir: true},
		{Path: "target/a.o", Ignored: true},
		{Path: "target/b.o", Ignored: true},
	}
	rows = Build(es, set("target"), false)
	want := []string{"0:dir:target", "1:file:a.o", "1:file:b.o"}
	if got := shape(rows); !slices.Equal(got, want) {
		t.Fatalf("Build = %q, want %q", got, want)
	}
	if !rows[0].Ignored || !rows[1].Ignored {
		t.Error("the rows under an ignored directory are not ignored, want ignored")
	}

	rows = Build([]Entry{{Path: "build.log", Ignored: true}}, nil, false)
	if !rows[0].Ignored || rows[0].File != 0 {
		t.Errorf("ignored file %+v, want an ignored file row", rows[0])
	}
}
