package import_e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ============================================================================
// Feature 1: Claude Code Detection & Parsing
// ============================================================================

func TestTier1_ClaudeCode_ProjectMcpJson(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpContent := `{
  "mcpServers": {
    "claude-tool": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-everything"],
      "env": {
        "DEBUG": "true"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "claude-tool") && !strings.Contains(stdout, "Successfully") {
		t.Errorf("expected success in stdout: %s", stdout)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv, exists := m.MCPServers["claude-tool"]
	if !exists {
		t.Fatalf("expected claude-tool server in manifest")
	}
	if srv.Command != "npx" || len(srv.Args) == 0 || srv.Args[0] != "-y" {
		t.Errorf("unexpected srv definition: %+v", srv)
	}
	if srv.Env["DEBUG"] != "true" {
		t.Errorf("expected env DEBUG=true, got: %v", srv.Env)
	}
}

func TestTier1_ClaudeCode_UserGlobalClaudeJson(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	claudeJSON := `{
  "mcpServers": {
    "global-claude-srv": {
      "command": "docker",
      "args": ["run", "-i", "--rm", "mcp/fetch"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(claudeJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["global-claude-srv"]; !exists {
		t.Errorf("expected global-claude-srv from ~/.claude.json in manifest")
	}
}

func TestTier1_ClaudeCode_ProjectSectionInUserClaudeJson(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	claudeJSON := `{
  "mcpServers": {
    "general-tool": { "command": "echo", "args": ["general"] }
  },
  "projects": {
    "` + projectPath + `": {
      "mcpServers": {
        "project-specific-claude": {
          "command": "python",
          "args": ["manage.py", "mcp"]
        }
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(claudeJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["project-specific-claude"]; !exists {
		t.Errorf("expected project-specific-claude from projects section in manifest")
	}
	if _, exists := m.MCPServers["general-tool"]; !exists {
		t.Errorf("expected general-tool from ~/.claude.json in manifest")
	}
}

func TestTier1_ClaudeCode_GlobalSettingsJson(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	settingsDir := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settingsJSON := `{
  "mcpServers": {
    "settings-claude-tool": {
      "command": "node",
      "args": ["server.js"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(settingsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["settings-claude-tool"]; !exists {
		t.Errorf("expected settings-claude-tool in manifest")
	}
}

func TestTier1_ClaudeCode_ClaudeSkillsDir(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	skillDir := filepath.Join(projectPath, ".claude", "skills", "code-review")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Code Review Skill\nInstructions here"), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.Skills) != 1 || m.Skills[0].Name != "code-review" {
		t.Fatalf("expected code-review skill in manifest, got: %+v", m.Skills)
	}

	mirroredMD := filepath.Join(projectPath, ".gandalf", "skills", "code-review", "SKILL.md")
	data, err := os.ReadFile(mirroredMD)
	if err != nil || !strings.Contains(string(data), "Code Review Skill") {
		t.Errorf("expected mirrored skill in .gandalf/skills: %v", err)
	}
}

func TestTier1_ClaudeCode_DisabledServerPreserved(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpContent := `{
  "mcpServers": {
    "disabled-srv": {
      "command": "echo",
      "disabled": true
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv, exists := m.MCPServers["disabled-srv"]
	if !exists || !srv.Disabled {
		t.Errorf("expected disabled-srv with disabled=true, got: %+v", srv)
	}
}

// ============================================================================
// Feature 2: Cursor Detection & Parsing
// ============================================================================

func TestTier1_Cursor_ProjectMcpJson(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	cursorJSON := `{
  "mcpServers": {
    "cursor-search": {
      "command": "ripgrep-mcp",
      "args": ["--context=2"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(cursorJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["cursor-search"]; !exists {
		t.Errorf("expected cursor-search in manifest")
	}
	if len(m.Agents) == 0 || m.Agents[0] != types.AgentCursor {
		t.Errorf("expected AgentCursor in agents list, got: %v", m.Agents)
	}
}

func TestTier1_Cursor_UserGlobalMcpJson(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	globalCursorDir := filepath.Join(homeDir, ".cursor")
	if err := os.MkdirAll(globalCursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	cursorJSON := `{
  "mcpServers": {
    "global-cursor-tool": {
      "command": "cursor-helper",
      "args": ["run"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(globalCursorDir, "mcp.json"), []byte(cursorJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["global-cursor-tool"]; !exists {
		t.Errorf("expected global-cursor-tool in manifest")
	}
}

func TestTier1_Cursor_EnvFileReference(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	cursorJSON := `{
  "mcpServers": {
    "envfile-srv": {
      "command": "node",
      "envFile": "${workspaceFolder}/.env.local"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(cursorJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["envfile-srv"]
	if srv.EnvFile != "${workspaceFolder}/.env.local" {
		t.Errorf("expected envFile to be preserved, got: %s", srv.EnvFile)
	}
}

func TestTier1_Cursor_CustomAuthStructure(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	cursorJSON := `{
  "mcpServers": {
    "sse-server": {
      "type": "sse",
      "url": "https://api.example.com/mcp",
      "auth": {
        "clientId": "my_client_id",
        "secret": "supersecretkey"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(cursorJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["sse-server"]
	if srv.Type != "sse" {
		t.Errorf("expected type sse, got: %s", srv.Type)
	}
	authMap, ok := srv.Auth.(map[string]any)
	if !ok {
		t.Fatalf("expected auth map[string]any, got: %T", srv.Auth)
	}
	if authMap["clientId"] != "my_client_id" {
		t.Errorf("expected clientId my_client_id, got: %v", authMap["clientId"])
	}
	// secret should be templatized
	secretVal, ok := authMap["secret"].(string)
	if !ok || !strings.Contains(secretVal, "${") {
		t.Errorf("expected secret to be templatized into ${...}, got: %v", authMap["secret"])
	}
}

func TestTier1_Cursor_CursorSkillsDir(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	skillDir := filepath.Join(projectPath, ".cursor", "skills", "git-assistant")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Git Assistant Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.Skills) != 1 || m.Skills[0].Name != "git-assistant" {
		t.Errorf("expected git-assistant skill, got: %+v", m.Skills)
	}
}

// ============================================================================
// Feature 3: Codex Detection & Parsing
// ============================================================================

func TestTier1_Codex_ProjectConfigToml(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	codexDir := filepath.Join(projectPath, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexTOML := `
[mcp_servers.codex-linter]
command = "ruff"
args = ["check", "--output-format=json"]
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexTOML), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv, exists := m.MCPServers["codex-linter"]
	if !exists {
		t.Fatalf("expected codex-linter in manifest")
	}
	if srv.Command != "ruff" {
		t.Errorf("expected ruff command, got: %s", srv.Command)
	}
}

func TestTier1_Codex_UserGlobalConfigToml(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	globalCodexDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(globalCodexDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexTOML := `
[mcp_servers.global-codex-srv]
command = "pytest"
args = ["-q"]
`
	if err := os.WriteFile(filepath.Join(globalCodexDir, "config.toml"), []byte(codexTOML), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["global-codex-srv"]; !exists {
		t.Errorf("expected global-codex-srv in manifest")
	}
}

func TestTier1_Codex_NestedEnvTable(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	codexDir := filepath.Join(projectPath, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexTOML := `
[mcp_servers.nested-env-tool]
command = "node"
args = ["index.js"]

[mcp_servers.nested-env-tool.env]
MODE = "strict"
RETRIES = "3"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexTOML), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv, exists := m.MCPServers["nested-env-tool"]
	if !exists {
		t.Fatalf("expected nested-env-tool in manifest")
	}
	if srv.Env["MODE"] != "strict" || srv.Env["RETRIES"] != "3" {
		t.Errorf("expected env vars folded into parent server, got: %v", srv.Env)
	}
	// Virtual server nested-env-tool.env should not exist
	if _, exists := m.MCPServers["nested-env-tool.env"]; exists {
		t.Errorf("virtual .env server should have been cleaned up")
	}
}

func TestTier1_Codex_DottedEnvKeys(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	codexDir := filepath.Join(projectPath, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexTOML := `
[mcp_servers.dotted-tool]
command = "python"
args = ["script.py"]
env.PORT = "8080"
env.HOST = "127.0.0.1"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexTOML), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv, exists := m.MCPServers["dotted-tool"]
	if !exists {
		t.Fatalf("expected dotted-tool in manifest")
	}
	if srv.Env["PORT"] != "8080" || srv.Env["HOST"] != "127.0.0.1" {
		t.Errorf("expected dotted env keys preserved, got: %v", srv.Env)
	}
}

func TestTier1_Codex_CodexSkillsDir(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	skillDir := filepath.Join(projectPath, ".codex", "skills", "py-refactor")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Python Refactoring Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.Skills) != 1 || m.Skills[0].Name != "py-refactor" {
		t.Errorf("expected py-refactor skill in manifest, got: %+v", m.Skills)
	}
}

// ============================================================================
// Feature 4: CLI Flags & Operational Modes
// ============================================================================

func TestTier1_CLIFlags_ProjectOnly(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Project source
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"local-srv":{"command":"node"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Global source
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(`{"mcpServers":{"global-srv":{"command":"python"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["local-srv"]; !exists {
		t.Errorf("expected local-srv to exist")
	}
	if _, exists := m.MCPServers["global-srv"]; exists {
		t.Errorf("expected global-srv to be excluded under --project-only")
	}
}

func TestTier1_CLIFlags_FromSpecificFile(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	customFile := filepath.Join(projectPath, "custom-mcp-config.json")
	if err := os.WriteFile(customFile, []byte(`{"mcpServers":{"custom-target":{"command":"go","args":["run","."]}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--from", customFile)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["custom-target"]; !exists {
		t.Errorf("expected custom-target server from --from flag")
	}
}

func TestTier1_CLIFlags_DryRun(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"dry-srv":{"command":"echo"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--dry-run")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	if !strings.Contains(stdout, "[DRY-RUN]") {
		t.Errorf("expected [DRY-RUN] in stdout, got: %s", stdout)
	}
	if !strings.Contains(stdout, "dry-srv") {
		t.Errorf("expected preview to contain dry-srv")
	}

	// gandalf.toml must not exist on disk
	if _, err := os.Stat(filepath.Join(projectPath, "gandalf.toml")); !os.IsNotExist(err) {
		t.Errorf("gandalf.toml was written to disk during dry run!")
	}
}

func TestTier1_CLIFlags_ForceOverwrite(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"new-srv":{"command":"echo"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(projectPath, "gandalf.toml")
	if err := os.WriteFile(manifestPath, []byte("existing content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Without --force: must fail safely
	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code == 0 {
		t.Fatalf("expected non-zero exit code when manifest exists without --force")
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("expected 'already exists' in stderr, got: %s", stderr)
	}

	// With --force: must succeed
	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--force")
	if code != 0 {
		t.Fatalf("expected success with --force, got code %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Successfully generated gandalf.toml") {
		t.Errorf("expected success message in stdout: %s", stdout)
	}
	m := assertManifestValid(t, projectPath, manifestPath)
	if _, exists := m.MCPServers["new-srv"]; !exists {
		t.Errorf("expected new-srv in overwritten manifest")
	}
}

func TestTier1_CLIFlags_JSONOutput(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"json-srv":{"command":"node"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--json")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	parsed := parseJSONOutput(t, stdout)
	if parsed["outputFile"] != "gandalf.toml" {
		t.Errorf("expected outputFile 'gandalf.toml', got: %v", parsed["outputFile"])
	}
	manifestMap, ok := parsed["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("expected manifest map in json output")
	}
	servers, ok := manifestMap["mcp_servers"].(map[string]any)
	if !ok || servers["json-srv"] == nil {
		t.Errorf("expected json-srv in manifest mcp_servers: %v", servers)
	}
}

func TestTier1_CLIFlags_CustomOutputFile(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"tool":{"command":"node"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	customOut := "custom-manifest.toml"
	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--output", customOut)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	customPath := filepath.Join(projectPath, customOut)
	if !fileExists(customPath) {
		t.Fatalf("expected output file '%s' to be created", customPath)
	}
	assertManifestValid(t, projectPath, customPath)
}

// ============================================================================
// Feature 5: Automated Secret Templatization
// ============================================================================

func TestTier1_Templatizer_DatabaseURLs(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "db-service": {
      "command": "npx",
      "args": ["-y", "db-mcp", "postgres://admin:s3cr3tp@ssword@db.corp.internal:5432/main_db"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["db-service"]
	if len(srv.Args) < 3 || strings.Contains(srv.Args[2], "s3cr3tp@ssword") {
		t.Fatalf("raw database credentials leaked in args: %v", srv.Args)
	}
	if !strings.Contains(srv.Args[2], "${DATABASE_URL}") {
		t.Errorf("expected ${DATABASE_URL} in args, got: %s", srv.Args[2])
	}
	if m.EnvTemplate["DATABASE_URL"] == "" || strings.Contains(m.EnvTemplate["DATABASE_URL"], "s3cr3t") {
		t.Errorf("expected safe placeholder in env_template, got: %s", m.EnvTemplate["DATABASE_URL"])
	}
}

func TestTier1_Templatizer_AnthropicAndOpenAIKeys(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "ai-agent": {
      "command": "python",
      "env": {
        "ANTHROPIC_KEY": "sk-ant-api03-0123456789abcdefghijklmnopqrstuvwxyz",
        "OPENAI_KEY": "sk-proj-0123456789abcdefghijklmnopqrstuvwxyz01"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["ai-agent"]
	for k, v := range srv.Env {
		if strings.Contains(v, "0123456789") {
			t.Errorf("raw key leaked in env %s: %s", k, v)
		}
		if !strings.HasPrefix(v, "${") {
			t.Errorf("expected ${VAR} templatization in env %s, got: %s", k, v)
		}
	}
}

func TestTier1_Templatizer_GitHubTokens(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "github-tool": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_1234567890abcdefghijklmnopqrstuvwxyz"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["github-tool"]
	envVal := srv.Env["GITHUB_PERSONAL_ACCESS_TOKEN"]
	if strings.Contains(envVal, "1234567890") {
		t.Errorf("raw github token leaked in manifest: %s", envVal)
	}
	if !strings.Contains(envVal, "${") {
		t.Errorf("expected ${...} templatization, got: %s", envVal)
	}
}

func TestTier1_Templatizer_BearerAndAuthHeaders(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "remote-api": {
      "type": "sse",
      "url": "https://api.internal.net/mcp",
      "headers": {
        "Authorization": "Bearer super-sensitive-jwt-token-value-abcdef123456",
        "X-Custom-Secret": "raw-secret-value-xyz"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["remote-api"]
	for k, v := range srv.Headers {
		if strings.Contains(v, "super-sensitive") || strings.Contains(v, "raw-secret") {
			t.Errorf("header %s leaked raw secret: %s", k, v)
		}
		if !strings.Contains(v, "${") {
			t.Errorf("expected header %s to be templatized, got: %s", k, v)
		}
	}
}

func TestTier1_Templatizer_FlagCredentials(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "cli-service": {
      "command": "npx",
      "args": [
        "--api-key=secret-apikey-12345678",
        "--auth-token",
        "positional-secret-auth-token-123"
      ]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["cli-service"]
	for _, arg := range srv.Args {
		if strings.Contains(arg, "secret-apikey") || strings.Contains(arg, "positional-secret") {
			t.Errorf("flag credential leaked into arg: %s", arg)
		}
	}
}

func TestTier1_Templatizer_SafeEnvTemplatePlaceholders(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "vault": {
      "command": "vault-tool",
      "env": {
        "DB_URI": "postgres://user:super_secret_db_pass@host:5432/db",
        "API_TOKEN": "sk-ant-api03-abcdef1234567890abcdef123456"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	for k, v := range m.EnvTemplate {
		if strings.Contains(v, "super_secret") || strings.Contains(v, "abcdef1234567890") {
			t.Errorf("env_template for %s exposed actual secret: %s", k, v)
		}
	}
}

// ============================================================================
// Feature 6: Multi-Source Precedence & Server Unification
// ============================================================================

func TestTier1_Precedence_ProjectOverridesGlobal(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Project has server "shared-srv"
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "shared-srv": { "command": "project-cmd", "args": ["project-arg"] }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Global has server "shared-srv" with different command
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(`{
  "mcpServers": {
    "shared-srv": { "command": "global-cmd", "args": ["global-arg"] }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["shared-srv"]
	if srv.Command != "project-cmd" || len(srv.Args) == 0 || srv.Args[0] != "project-arg" {
		t.Errorf("expected project-level definition to take precedence over global, got: %+v", srv)
	}
}

func TestTier1_Precedence_IdenticalServersAcrossAgents(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Claude Code config
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "shared-tool": { "command": "npx", "args": ["shared-tool"] }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Cursor config with same server
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(`{
  "mcpServers": {
    "shared-tool": { "command": "npx", "args": ["shared-tool"] }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	// Unified under single definition
	if len(m.MCPServers) != 1 {
		t.Errorf("expected exactly 1 unified server, got: %d", len(m.MCPServers))
	}
	if _, exists := m.MCPServers["shared-tool"]; !exists {
		t.Errorf("expected shared-tool server in manifest")
	}
}

func TestTier1_Precedence_DistinctServersUnion(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Claude server
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": { "claude-tool": { "command": "echo" } }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Cursor server
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(`{
  "mcpServers": { "cursor-tool": { "command": "node" } }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Codex server
	codexDir := filepath.Join(projectPath, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`
[mcp_servers.codex-tool]
command = "python"
`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.MCPServers) != 3 {
		t.Fatalf("expected 3 distinct servers, got: %d", len(m.MCPServers))
	}
	for _, name := range []string{"claude-tool", "cursor-tool", "codex-tool"} {
		if _, exists := m.MCPServers[name]; !exists {
			t.Errorf("expected server %s in manifest", name)
		}
	}
}

func TestTier1_Precedence_TargetAgentsUnion(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"c":{"command":"echo"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(`{"mcpServers":{"cu":{"command":"echo"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	hasClaude := false
	hasCursor := false
	for _, a := range m.Agents {
		if a == types.AgentClaudeCode {
			hasClaude = true
		}
		if a == types.AgentCursor {
			hasCursor = true
		}
	}
	if !hasClaude || !hasCursor {
		t.Errorf("expected both claude-code and cursor in target agents, got: %v", m.Agents)
	}
}

func TestTier1_Precedence_DeterministicSortedOutput(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Create servers out of alphabetical order
	mcpJSON := `{
  "mcpServers": {
    "zebra": { "command": "z" },
    "alpha": { "command": "a" },
    "beta": { "command": "b" },
    "gamma": { "command": "g" }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(manifestBytes)

	posAlpha := strings.Index(content, "[mcp_servers.alpha]")
	posBeta := strings.Index(content, "[mcp_servers.beta]")
	posGamma := strings.Index(content, "[mcp_servers.gamma]")
	posZebra := strings.Index(content, "[mcp_servers.zebra]")

	if !(posAlpha < posBeta && posBeta < posGamma && posGamma < posZebra) {
		t.Errorf("expected deterministic alphabetical sorting of servers: alpha(%d) < beta(%d) < gamma(%d) < zebra(%d)",
			posAlpha, posBeta, posGamma, posZebra)
	}
}

// ============================================================================
// Feature 7: Team Skill Scanning & Safe Mirroring
// ============================================================================

func TestTier1_Skills_ScanValidSkillMD(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	skillDir := filepath.Join(projectPath, ".cursor", "skills", "my-valid-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Valid Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.Skills) != 1 || m.Skills[0].Name != "my-valid-skill" {
		t.Errorf("expected my-valid-skill in manifest: %+v", m.Skills)
	}
}

func TestTier1_Skills_IgnoreFoldersWithoutSkillMD(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	skillsDir := filepath.Join(projectPath, ".cursor", "skills")
	// Valid skill
	if err := os.MkdirAll(filepath.Join(skillsDir, "valid-skill"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "valid-skill", "SKILL.md"), []byte("# Valid"), 0644); err != nil {
		t.Fatal(err)
	}
	// Invalid folder (no SKILL.md)
	if err := os.MkdirAll(filepath.Join(skillsDir, "scratch-dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "scratch-dir", "notes.txt"), []byte("notes"), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.Skills) != 1 || m.Skills[0].Name != "valid-skill" {
		t.Errorf("expected only valid-skill in manifest: %+v", m.Skills)
	}
	if _, err := os.Stat(filepath.Join(projectPath, ".gandalf", "skills", "scratch-dir")); !os.IsNotExist(err) {
		t.Errorf("scratch-dir was erroneously mirrored into .gandalf/skills")
	}
}

func TestTier1_Skills_ProjectSkillOverridesGlobalSkill(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Global skill
	globalSkill := filepath.Join(homeDir, ".claude", "skills", "review")
	if err := os.MkdirAll(globalSkill, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalSkill, "SKILL.md"), []byte("# Global Review"), 0644); err != nil {
		t.Fatal(err)
	}

	// Project skill with same name
	projSkill := filepath.Join(projectPath, ".cursor", "skills", "review")
	if err := os.MkdirAll(projSkill, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projSkill, "SKILL.md"), []byte("# Project Review"), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--force")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	mirrored := filepath.Join(projectPath, ".gandalf", "skills", "review", "SKILL.md")
	data, err := os.ReadFile(mirrored)
	if err != nil {
		t.Fatalf("mirrored skill missing: %v", err)
	}
	if !strings.Contains(string(data), "# Project Review") {
		t.Errorf("expected project skill to override global, got content: %s", string(data))
	}
}

func TestTier1_Skills_PreserveExecutablePermissions(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	skillDir := filepath.Join(projectPath, ".cursor", "skills", "runner")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Runner"), 0644); err != nil {
		t.Fatal(err)
	}
	execScript := filepath.Join(skillDir, "run.sh")
	if err := os.WriteFile(execScript, []byte("#!/bin/sh\necho ok"), 0755); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	destScript := filepath.Join(projectPath, ".gandalf", "skills", "runner", "run.sh")
	fi, err := os.Stat(destScript)
	if err != nil {
		t.Fatalf("mirrored script not found: %v", err)
	}
	if fi.Mode().Perm()&0111 == 0 {
		t.Errorf("expected executable bit preserved on %s, got mode: %v", destScript, fi.Mode())
	}
}

func TestTier1_Skills_ForceCleansObsoleteFiles(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	srcSkillDir := filepath.Join(projectPath, ".cursor", "skills", "clean-test")
	if err := os.MkdirAll(srcSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkillDir, "SKILL.md"), []byte("# Clean Test New"), 0644); err != nil {
		t.Fatal(err)
	}

	// Existing target skill has obsolete extra file
	destSkillDir := filepath.Join(projectPath, ".gandalf", "skills", "clean-test")
	if err := os.MkdirAll(destSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destSkillDir, "SKILL.md"), []byte("# Clean Test Old"), 0644); err != nil {
		t.Fatal(err)
	}
	obsoleteFile := filepath.Join(destSkillDir, "obsolete_extra.js")
	if err := os.WriteFile(obsoleteFile, []byte("console.log('old');"), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only", "--force")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	if _, err := os.Stat(obsoleteFile); !os.IsNotExist(err) {
		t.Errorf("expected obsolete file to be removed when overwriting with --force")
	}
}

func TestTier1_Skills_AtomicRollbackOnManifestWriteFailure(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	srcSkillDir := filepath.Join(projectPath, ".cursor", "skills", "rollback-skill")
	if err := os.MkdirAll(srcSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkillDir, "SKILL.md"), []byte("# Rollback Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Make OutputFile point to a directory that cannot be written as a file to force write failure
	opts := importer.ImportOptions{
		ProjectPath: projectPath,
		HomeDir:     homeDir,
		ProjectOnly: true,
		OutputFile:  ".cursor", // directory, WriteTextAtomically fails!
	}

	_, err := runImportDirect(t, opts)
	if err == nil {
		t.Fatalf("expected RunImport to fail on directory output file")
	}

	// Newly mirrored skill must be rolled back
	destSkillDir := filepath.Join(projectPath, ".gandalf", "skills", "rollback-skill")
	if _, statErr := os.Stat(destSkillDir); !os.IsNotExist(statErr) {
		t.Errorf("expected newly mirrored skill to be rolled back after failure")
	}
}

func TestTier1_ImportAlias_SamePathAsExport(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpContent := `{"mcpServers": {"alias-tool": {"command": "npx"}}}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpContent), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "import", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import alias failed: %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Successfully generated gandalf.toml") {
		t.Errorf("expected success from import alias, got: %s", stdout)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["alias-tool"]; !exists {
		t.Fatalf("expected alias-tool server from import alias")
	}
}
