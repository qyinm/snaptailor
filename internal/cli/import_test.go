package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestCLIExport_Success(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Create project-level .mcp.json
	mcpJSON := `{
  "mcpServers": {
    "test-server": {
      "command": "npx",
      "args": ["-y", "@mcp/test", "postgres://user:pass@localhost:5432/mydb"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}

	if !strings.Contains(stdout, "Successfully generated gandalf.toml") {
		t.Errorf("expected success message in stdout, got: %s", stdout)
	}

	// Verify gandalf.toml was created and contains templated DATABASE_URL
	manifestData, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatalf("failed to read generated gandalf.toml: %v", err)
	}

	manifestContent := string(manifestData)
	if !strings.Contains(manifestContent, "[mcp_servers.test-server]") {
		t.Errorf("expected test-server in gandalf.toml")
	}
	if !strings.Contains(manifestContent, "${DATABASE_URL}") {
		t.Errorf("expected secret URL to be replaced with ${DATABASE_URL}")
	}
	if !strings.Contains(manifestContent, "[env_template]") {
		t.Errorf("expected [env_template] section in gandalf.toml")
	}
}

func TestCLIExport_DryRun(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"demo": {"command": "node"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
		"--dry-run",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}

	if !strings.Contains(stdout, "[DRY-RUN]") {
		t.Errorf("expected [DRY-RUN] in stdout, got: %s", stdout)
	}

	// Ensure no file was written
	if _, err := os.Stat(filepath.Join(projectPath, "gandalf.toml")); !os.IsNotExist(err) {
		t.Errorf("expected gandalf.toml NOT to exist on dry-run")
	}
}

func TestCLIExport_JSONOutput(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"json-tool": {"command": "npx"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
		"--json",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("failed to parse json output: %v. Raw stdout: %s", err, stdout)
	}

	if parsed["outputFile"] != "gandalf.toml" {
		t.Errorf("expected outputFile 'gandalf.toml', got %v", parsed["outputFile"])
	}
}

func TestCLIExport_ForceOverwrite(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"tool": {"command": "node"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create pre-existing gandalf.toml
	if err := os.WriteFile(filepath.Join(projectPath, "gandalf.toml"), []byte("existing content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Running without --force should fail
	_, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
	)

	if code == 0 {
		t.Fatalf("expected non-zero exit code when manifest exists without --force")
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("expected 'already exists' in stderr, got: %s", stderr)
	}

	// Running with --force should succeed
	stdout, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
		"--force",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0 with --force, got %d. Stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Successfully generated gandalf.toml") {
		t.Errorf("expected success with --force, got: %s", stdout)
	}
}

func TestCLIExport_ProjectOnly(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Project-level .cursor/mcp.json
	if err := os.MkdirAll(filepath.Join(projectPath, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	projMCP := `{"mcpServers": {"proj-server": {"command": "npx"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".cursor", "mcp.json"), []byte(projMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// Global-level ~/.cursor/mcp.json
	if err := os.MkdirAll(filepath.Join(homeDir, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	globalMCP := `{"mcpServers": {"global-server": {"command": "python"}}}`
	if err := os.WriteFile(filepath.Join(homeDir, ".cursor", "mcp.json"), []byte(globalMCP), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
		"--project-only",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}

	content, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "proj-server") {
		t.Errorf("expected proj-server in manifest")
	}
	if strings.Contains(string(content), "global-server") {
		t.Errorf("expected global-server to be excluded with --project-only, stdout: %s", stdout)
	}
}

func cliLaunchImportTUIForTest(t *testing.T, fn func(types.RuntimeOptions, importer.ImportOptions) int) func() {
	t.Helper()
	prev := launchImportTUI
	launchImportTUI = fn
	return func() {
		launchImportTUI = prev
	}
}

func TestShouldLaunchImportTUI(t *testing.T) {
	t.Parallel()
	withJSON := func(f *importFlags) { f.JSON = true }
	withDryRun := func(f *importFlags) { f.DryRun = true }
	withForce := func(f *importFlags) { f.Force = true }
	withInteractive := func(f *importFlags) { f.Interactive = true }
	withInteractiveAndDryRun := func(f *importFlags) { f.Interactive = true; f.DryRun = true }
	withInteractiveAndJSON := func(f *importFlags) { f.Interactive = true; f.JSON = true }

	cases := []struct {
		name   string
		mutate func(*importFlags)
		isTTY  bool
		want   bool
	}{
		{"interactive default", nil, true, true},
		{"interactive with force", withForce, true, true},
		{"json stays headless", withJSON, true, false},
		{"dry-run stays headless", withDryRun, true, false},
		{"piped output stays headless", nil, false, false},
		{"piped json stays headless", withJSON, false, false},
		{"interactive flag overrides non-TTY", withInteractive, false, true},
		{"interactive flag with TTY", withInteractive, true, true},
		{"interactive flag with dry-run stays headless", withInteractiveAndDryRun, false, false},
		{"interactive flag with json stays headless", withInteractiveAndJSON, false, false},
		{"interactive flag with dry-run and TTY stays headless", withInteractiveAndDryRun, true, false},
		{"interactive flag with json and TTY stays headless", withInteractiveAndJSON, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := importFlags{}
			if tc.mutate != nil {
				tc.mutate(&flags)
			}
			if got := shouldLaunchImportTUI(&flags, tc.isTTY); got != tc.want {
				t.Errorf("shouldLaunchImportTUI(%+v, %v) = %v, want %v", flags, tc.isTTY, got, tc.want)
			}
		})
	}
}

func TestRunCLIExport_DoesNotLaunchTUIWhenPiped(t *testing.T) {
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"piped-server": {"command": "npx"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Stub the TUI launch seam: it must NOT be called for captured (non-TTY)
	// output, otherwise automation would block on an interactive screen.
	launched := false
	restore := cliLaunchImportTUIForTest(t, func(_ types.RuntimeOptions, _ importer.ImportOptions) int {
		launched = true
		return 0
	})
	defer restore()

	_, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
	)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}
	if launched {
		t.Errorf("TUI must not launch when stdout is not a terminal")
	}
}

func TestCLIExport_InteractiveFlag_LaunchesTUI(t *testing.T) {
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"interactive-server": {"command": "npx"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	called := false
	var capturedOpts importer.ImportOptions
	restore := cliLaunchImportTUIForTest(t, func(_ types.RuntimeOptions, opts importer.ImportOptions) int {
		called = true
		capturedOpts = opts
		return 0
	})
	defer restore()

	stdout, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
		"--interactive",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0 with --interactive stub, got %d. Stderr: %s", code, stderr)
	}
	if !called {
		t.Errorf("expected launchImportTUI to be called when --interactive is provided, stdout: %s", stdout)
	}
	realProject := projectPath
	if rp, err := filepath.EvalSymlinks(projectPath); err == nil {
		realProject = rp
	}
	if capturedOpts.ProjectPath != realProject {
		t.Errorf("expected ProjectPath %q, got %q", realProject, capturedOpts.ProjectPath)
	}
}

func TestCLIExport_InteractiveFlag_DryRunOverridesTUI(t *testing.T) {
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"demo": {"command": "node"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	called := false
	restore := cliLaunchImportTUIForTest(t, func(_ types.RuntimeOptions, _ importer.ImportOptions) int {
		called = true
		return 0
	})
	defer restore()

	stdout, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
		"--interactive",
		"--dry-run",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}
	if called {
		t.Errorf("launchImportTUI must NOT be called when --dry-run is passed even with --interactive")
	}
	if !strings.Contains(stdout, "[DRY-RUN]") {
		t.Errorf("expected [DRY-RUN] in stdout, got: %s", stdout)
	}
}

func TestCLIExport_InteractiveFlag_JSONOverridesTUI(t *testing.T) {
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"demo": {"command": "node"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	called := false
	restore := cliLaunchImportTUIForTest(t, func(_ types.RuntimeOptions, _ importer.ImportOptions) int {
		called = true
		return 0
	})
	defer restore()

	stdout, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
		"-i",
		"--json",
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}
	if called {
		t.Errorf("launchImportTUI must NOT be called when --json is passed even with -i")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("expected valid json output, got: %s", stdout)
	}
}

func TestCLIExport_CustomOutput(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"custom-output-srv": {"command": "node"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	customOut := "custom-manifest.toml"
	stdout, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
		"-o", customOut,
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Successfully generated "+customOut) {
		t.Errorf("expected output mentioning %s, got: %s", customOut, stdout)
	}
	if _, err := os.Stat(filepath.Join(projectPath, customOut)); err != nil {
		t.Fatalf("expected %s to be created on disk: %v", customOut, err)
	}
}

func TestCLIExport_FromFlag(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Create custom config outside standard discovery paths
	customDir := t.TempDir()
	customPath := filepath.Join(customDir, "explicit-mcp.json")
	customContent := `{"mcpServers": {"explicit-server": {"command": "python", "args": ["main.py"]}}}`
	if err := os.WriteFile(customPath, []byte(customContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t,
		"export",
		"--project", projectPath,
		"--home", homeDir,
		"--from", customPath,
	)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}

	manifestData, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatalf("failed to read generated gandalf.toml: %v", err)
	}
	if !strings.Contains(string(manifestData), "explicit-server") {
		t.Errorf("expected explicit-server from --from file, got content: %s", string(manifestData))
	}
}

func TestCLIExport_HelpIsPrimaryAndDocumentsImportAlias(t *testing.T) {
	t.Parallel()

	stdout, stderr, code := runCLI(t, "export", "--help")
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Export local agent environment to gandalf.toml.") {
		t.Errorf("expected export short help, got: %s", stdout)
	}
	if !strings.Contains(stdout, "gandalf export") {
		t.Errorf("expected primary usage gandalf export, got: %s", stdout)
	}
	if !strings.Contains(stdout, "import") {
		t.Errorf("expected import alias documented in export help, got: %s", stdout)
	}
	if !strings.Contains(stdout, "backward-compatible alias") {
		t.Errorf("expected alias guidance in long help, got: %s", stdout)
	}
	if !strings.Contains(stdout, "gandalf apply") {
		t.Errorf("expected apply contrast in long help, got: %s", stdout)
	}
}

func TestCLIRootHelp_ListsExportNotImportAsPrimary(t *testing.T) {
	t.Parallel()

	stdout, stderr, code := runCLI(t, "--help")
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "export") {
		t.Errorf("expected export in root command list, got: %s", stdout)
	}
	// Cobra lists the primary Use name, not aliases, under Available Commands.
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") || trimmed == "import" {
			t.Errorf("import must not appear as a primary command in root help: %q", line)
		}
	}
}

func TestCLIImport_AliasRunsSamePath(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{"mcpServers": {"alias-server": {"command": "npx"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t,
		"import",
		"--project", projectPath,
		"--home", homeDir,
	)
	if code != 0 {
		t.Fatalf("expected import alias to succeed, got %d. Stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Successfully generated gandalf.toml") {
		t.Errorf("expected success message from import alias, got: %s", stdout)
	}

	manifestData, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatalf("failed to read generated gandalf.toml: %v", err)
	}
	if !strings.Contains(string(manifestData), "alias-server") {
		t.Errorf("expected alias-server in gandalf.toml")
	}
}

func TestCLIImport_HelpResolvesToExport(t *testing.T) {
	t.Parallel()

	stdout, stderr, code := runCLI(t, "import", "--help")
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "gandalf export") {
		t.Errorf("import --help should show export as the primary command, got: %s", stdout)
	}
	if !strings.Contains(stdout, "backward-compatible alias") {
		t.Errorf("import --help should document the alias, got: %s", stdout)
	}
}
