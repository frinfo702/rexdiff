package test

import (
	"os"
	"testing"

	"github.com/rednafi/rexdiff/internal/testgit"
)

var (
	home     string // the HOME of the user, for the real Rex
	buildDir string // the directory of the binaries that the tests build
)

func TestMain(m *testing.M) {
	home = os.Getenv("HOME")
	var err error
	if buildDir, err = os.MkdirTemp("", "rexdiff-test"); err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(buildDir) }()
	testgit.Main(m)
}
