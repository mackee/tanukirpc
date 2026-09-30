//go:build tsintegration

package genclient_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// These tests run the generated TypeScript clients through the toolchain
// pinned in testdata/tstest/package.json. They are opt-in because they need
// Node.js and npm:
//
//	(cd genclient/testdata/tstest && npm ci)
//	go test -tags tsintegration ./genclient/

// TestGoldenClientsTypeCheck type-checks every client.ts golden so we catch
// generation-time regressions that produce invalid TypeScript — including
// issues a linter would flag (the `useLiteralKeys` rule's bracket-notation
// suggestion, undeclared identifiers, etc.).
func TestGoldenClientsTypeCheck(t *testing.T) {
	runTSTestScript(t, "typecheck")
}

// TestGeneratedClientRuntime runs the node:test suites under
// testdata/tstest/src against the generated clients.
func TestGeneratedClientRuntime(t *testing.T) {
	runTSTestScript(t, "test:runtime")
}

func runTSTestScript(t *testing.T, script string) {
	t.Helper()
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		t.Fatalf("npm not on PATH: %v", err)
	}
	dir := filepath.Join(analysistest.TestData(), "tstest")
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err != nil {
		t.Fatalf("TypeScript toolchain not installed; run `npm ci` in %s", dir)
	}
	cmd := exec.Command(npmPath, "run", "--silent", script)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("npm run %s failed:\n%s", script, strings.TrimSpace(string(out)))
	}
}
