package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain lets the scripts run rexdiff, and a fake rex, as commands. Each
// command is a copy of the test binary that calls the function here.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"rexdiff": main,
		"rex":     fakeRex,
	})
}

// fakeRex writes its arguments to $WORK/rex.log, so that a script can see
// what rexdiff asked Rex to do. It fails if $REX_FAIL is set.
func fakeRex() {
	log := filepath.Join(os.Getenv("WORK"), "rex.log")
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(f, strings.Join(os.Args[1:], " "))
	f.Close()
	if os.Getenv("REX_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "no Rex server")
		os.Exit(1)
	}
	fmt.Println("config reloaded")
}

// userEnv lists the variables of the user that change what rexdiff and rex
// do. Each script starts with them empty.
var userEnv = []string{
	"REX_BIN_DIR",
	"REX_CONFIG",
	"REX_SESSION",
	"REX_BLOCK",
	"REX_AGENT_BLOCK",
	"REXDIFF_LOG",
	"REXDIFF_RESUME",
}

// TestScripts runs the scripts in testdata/script.
func TestScripts(t *testing.T) {
	// If the real rex tool is installed, $REALREX is its path, and the
	// condition [realrex] is true.
	realRex := filepath.Join(os.Getenv("REX_BIN_DIR"), "rex")
	_, err := os.Stat(realRex)
	hasRex := os.Getenv("REX_BIN_DIR") != "" && err == nil
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
		RequireExplicitExec: true,
		Setup: func(env *testscript.Env) error {
			// Each script runs in a new directory, $WORK. $WORK is also
			// $HOME, so rexdiff does not see the files of the user.
			env.Setenv("HOME", env.WorkDir)
			env.Setenv("XDG_CONFIG_HOME", "")
			env.Setenv("REALREX", realRex)
			for _, k := range userEnv {
				env.Setenv(k, "")
			}
			return nil
		},
		Condition: func(cond string) (bool, error) {
			if cond == "realrex" {
				return hasRex, nil
			}
			return false, fmt.Errorf("unknown condition %q", cond)
		},
	})
}
