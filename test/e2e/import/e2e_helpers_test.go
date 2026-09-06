package import_e2e_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/cli"
	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// makeSandbox creates an isolated project directory, home directory, and store directory.
func makeSandbox(t *testing.T) (projectPath, homeDir, storeDir string) {
	t.Helper()
	root := t.TempDir()
	projectPath = filepath.Join(root, "project")
	homeDir = filepath.Join(root, "home")
	storeDir = filepath.Join(homeDir, ".gandalf")

	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("failed to create project directory: %v", err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("failed to create home directory: %v", err)
	}

	if resolved, err := filepath.EvalSymlinks(projectPath); err == nil {
		projectPath = resolved
	}
	if resolved, err := filepath.EvalSymlinks(homeDir); err == nil {
		homeDir = resolved
	}
	return projectPath, homeDir, storeDir

}

// runCLI executes the gandalf CLI command with specified arguments and returns stdout, stderr, and exitCode.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := cli.NewRootCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	exitCode = 0
	if err != nil {
		if code, ok := cli.IsExitError(err); ok {
			exitCode = code
		} else {
			exitCode = 1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// assertManifestValid parses and validates the generated gandalf.toml, asserting 0 validation errors.
func assertManifestValid(t *testing.T, projectPath, manifestPath string) *manifest.Manifest {
	t.Helper()
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest at '%s': %v", manifestPath, err)
	}

	parsed, err := manifest.Parse(string(data), &manifest.ParseOptions{NoInterpolate: true})
	if err != nil {
		t.Fatalf("failed to parse manifest at '%s': %v\nContent:\n%s", manifestPath, err, string(data))
	}

	valErrs := manifest.Validate(parsed.Manifest, projectPath)
	if len(valErrs) > 0 {
		var errMsgs []string
		for _, ve := range valErrs {
			errMsgs = append(errMsgs, ve.Error())
		}
		t.Fatalf("manifest validation failed with %d errors: %s\nContent:\n%s",
			len(valErrs), strings.Join(errMsgs, "; "), string(data))
	}
	return parsed.Manifest
}

// assertCheckInSync executes 'gandalf check --project-only --ci' and verifies InSync is true (exit code 0).
func assertCheckInSync(t *testing.T, projectPath string) {
	t.Helper()
	stdout, stderr, code := runCLI(t,
		"check",
		"--project", projectPath,
		"--project-only",
		"--ci",
	)
	if code != 0 {
		t.Fatalf("expected 'gandalf check --project-only --ci' to succeed with exit code 0, got %d.\nStdout: %s\nStderr: %s",
			code, stdout, stderr)
	}
	if !strings.Contains(stdout, "IN SYNC") {
		t.Errorf("expected 'IN SYNC' in check output, got: %s", stdout)
	}
}

// parseJSONOutput parses stdout into a map for --json assertions.
func parseJSONOutput(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v\nRaw stdout: %s", err, stdout)
	}
	return out
}

// runImportDirect invokes the importer core API directly.
func runImportDirect(t *testing.T, opts importer.ImportOptions) (*importer.ImportResult, error) {
	t.Helper()
	return importer.RunImport(opts)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// syncProjectAgentConfigs aligns project agent configuration files with the canonical manifest.
// This represents initializing a repository with the exported team manifest so that
// native agent files mirror the reconciled manifest.
func syncProjectAgentConfigs(t *testing.T, projectPath string, m *manifest.Manifest) {
	t.Helper()
	jsonBytes, err := json.MarshalIndent(map[string]any{"mcpServers": m.MCPServers}, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal mcpServers: %v", err)
	}

	for _, a := range m.Agents {
		switch a {
		case types.AgentClaudeCode:
			if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), jsonBytes, 0644); err != nil {
				t.Fatalf("failed to sync .mcp.json: %v", err)
			}
		case types.AgentCursor:
			cursorDir := filepath.Join(projectPath, ".cursor")
			if err := os.MkdirAll(cursorDir, 0755); err != nil {
				t.Fatalf("failed to create .cursor dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), jsonBytes, 0644); err != nil {
				t.Fatalf("failed to sync .cursor/mcp.json: %v", err)
			}
		case types.AgentCodex:
			codexDir := filepath.Join(projectPath, ".codex")
			if err := os.MkdirAll(codexDir, 0755); err != nil {
				t.Fatalf("failed to create .codex dir: %v", err)
			}
			tomlStr := importer.FormatManifestTOML(m)
			if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(tomlStr), 0644); err != nil {
				t.Fatalf("failed to sync .codex/config.toml: %v", err)
			}
		}
	}
}


