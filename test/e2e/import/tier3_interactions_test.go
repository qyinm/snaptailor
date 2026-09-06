package import_e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ============================================================================
// Tier 3: Cross-Feature Interactions
// ============================================================================

// Interaction 1: Multi-Agent + Secrets + Project-Only
func TestTier3_Interaction_MultiAgent_Secrets_ProjectOnly(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// 1. Claude Code project config with Database URL
	claudeMCP := `{
  "mcpServers": {
    "postgres-db": {
      "command": "npx",
      "args": ["-y", "@mcp/postgres", "postgres://user:secretpw@10.0.0.1:5432/proddb"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(claudeMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Cursor project config with Bearer token header
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	cursorMCP := `{
  "mcpServers": {
    "remote-gateway": {
      "type": "sse",
      "url": "https://gateway.internal.net/sse",
      "headers": {
        "Authorization": "Bearer super-jwt-token-abcdef123456"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(cursorMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Global config that should be IGNORED under --project-only
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(`{"mcpServers":{"global-srv":{"command":"python"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Successfully") {
		t.Errorf("expected success message in stdout: %s", stdout)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))

	// Verify both project servers are present
	if _, exists := m.MCPServers["postgres-db"]; !exists {
		t.Errorf("expected postgres-db server in manifest")
	}
	if _, exists := m.MCPServers["remote-gateway"]; !exists {
		t.Errorf("expected remote-gateway server in manifest")
	}

	// Verify global server was excluded
	if _, exists := m.MCPServers["global-srv"]; exists {
		t.Errorf("expected global-srv to be excluded under --project-only")
	}

	// Verify both agents recorded
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
		t.Errorf("expected both claude-code and cursor in agents list: %v", m.Agents)
	}

	// Verify secret redactions
	dbSrv := m.MCPServers["postgres-db"]
	if strings.Contains(dbSrv.Args[2], "secretpw") {
		t.Errorf("raw DB password leaked into args: %v", dbSrv.Args)
	}
	if !strings.Contains(dbSrv.Args[2], "${DATABASE_URL}") {
		t.Errorf("expected ${DATABASE_URL} in args, got: %s", dbSrv.Args[2])
	}

	gwSrv := m.MCPServers["remote-gateway"]
	if strings.Contains(gwSrv.Headers["Authorization"], "super-jwt-token") {
		t.Errorf("raw JWT token leaked into headers: %v", gwSrv.Headers)
	}

	// Verify env_template contains declarations for both secrets
	if m.EnvTemplate["DATABASE_URL"] == "" {
		t.Errorf("expected DATABASE_URL declared in env_template")
	}
	foundAuth := false
	for k := range m.EnvTemplate {
		if strings.Contains(k, "REMOTE_GATEWAY") || strings.Contains(k, "AUTH") {
			foundAuth = true
			break
		}
	}
	if !foundAuth {
		t.Errorf("expected remote-gateway auth token declared in env_template: %v", m.EnvTemplate)
	}
}

// Interaction 2: Multi-Agent + Secrets + Dry-Run + JSON
func TestTier3_Interaction_MultiAgent_Secrets_DryRun_JSON(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Claude Code config
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "claude-ai": {
      "command": "python",
      "env": { "ANTHROPIC_API_KEY": "sk-ant-api03-0123456789abcdefghijklmnopqrstuvwxyz" }
    }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Codex config
	codexDir := filepath.Join(projectPath, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`
[mcp_servers.codex-ai]
command = "node"
env.OPENAI_API_KEY = "sk-proj-0123456789abcdefghijklmnopqrstuvwxyz01"
`), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	// No file written on disk
	if _, err := os.Stat(filepath.Join(projectPath, "gandalf.toml")); !os.IsNotExist(err) {
		t.Fatalf("file gandalf.toml was written during dry-run + json")
	}

	parsed := parseJSONOutput(t, stdout)
	if parsed["dryRun"] != true {
		t.Errorf("expected dryRun: true in JSON output")
	}

	manifestMap, ok := parsed["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("expected manifest object in JSON output")
	}
	servers, ok := manifestMap["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcp_servers map in JSON output")
	}
	if servers["claude-ai"] == nil || servers["codex-ai"] == nil {
		t.Errorf("expected both claude-ai and codex-ai in JSON manifest: %v", servers)
	}

	extractedEnvs, ok := parsed["extractedEnvs"].(map[string]any)
	if !ok {
		t.Fatalf("expected extractedEnvs in JSON output")
	}
	if len(extractedEnvs) < 2 {
		t.Errorf("expected at least 2 extracted envs, got: %v", extractedEnvs)
	}
}

// Interaction 3: Explicit --from + Secrets + Force Overwrite
func TestTier3_Interaction_ExplicitFrom_Secrets_ForceOverwrite(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Pre-existing gandalf.toml
	manifestPath := filepath.Join(projectPath, "gandalf.toml")
	if err := os.WriteFile(manifestPath, []byte("old-content-here"), 0644); err != nil {
		t.Fatal(err)
	}

	// Separate source file outside standard paths
	customDir := filepath.Join(projectPath, "vendor_agent")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		t.Fatal(err)
	}
	customFile := filepath.Join(customDir, "custom.json")
	if err := os.WriteFile(customFile, []byte(`{
  "mcpServers": {
    "custom-agent": {
      "command": "custom-cli",
      "args": ["--token", "ghp_0123456789abcdefghijklmnopqrstuvwxyz"]
    }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--from", customFile, "--force")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, manifestPath)
	if _, ok := m.MCPServers["custom-agent"]; !ok {
		t.Errorf("expected custom-agent in overwritten manifest")
	}
	srv := m.MCPServers["custom-agent"]
	if strings.Contains(srv.Args[1], "ghp_012345") {
		t.Errorf("raw token leaked into args: %v", srv.Args)
	}
	if !strings.Contains(srv.Args[1], "${") {
		t.Errorf("expected token in args to be templatized: %v", srv.Args)
	}
}

// Interaction 4: Multi-Agent + Skills + Project-Only + Force Overwrite
func TestTier3_Interaction_MultiAgent_Skills_ProjectOnly_Force(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// 1. Claude skills
	claudeSkills := filepath.Join(projectPath, ".claude", "skills", "claude-skill")
	if err := os.MkdirAll(claudeSkills, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeSkills, "SKILL.md"), []byte("# Claude Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Cursor skills
	cursorSkills := filepath.Join(projectPath, ".cursor", "skills", "cursor-skill")
	if err := os.MkdirAll(cursorSkills, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorSkills, "SKILL.md"), []byte("# Cursor Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Pre-create one destination team skill
	destDir := filepath.Join(projectPath, ".gandalf", "skills", "claude-skill")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "SKILL.md"), []byte("# Old Claude Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run with --force and --project-only
	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only", "--force")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.Skills) != 2 {
		t.Fatalf("expected 2 skills in manifest, got: %d", len(m.Skills))
	}

	// Overwrite should have applied new content
	updatedMD, err := os.ReadFile(filepath.Join(destDir, "SKILL.md"))
	if err != nil || !strings.Contains(string(updatedMD), "# Claude Skill") {
		t.Errorf("expected updated claude skill after force overwrite, got: %s", string(updatedMD))
	}
}

// Interaction 5: Multi-Source Precedence Conflict + Dotted Server Names + JSON Output
func TestTier3_Interaction_PrecedenceConflict_DottedNames_JSON(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Global config with dotted name
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(`{
  "mcpServers": {
    "org.service.router": { "command": "global-router" }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Project config with same dotted name overriding global
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "org.service.router": { "command": "project-router", "args": ["--mode=strict"] }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--json")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	parsed := parseJSONOutput(t, stdout)
	manifestMap := parsed["manifest"].(map[string]any)
	servers := manifestMap["mcp_servers"].(map[string]any)
	routerMap, ok := servers["org.service.router"].(map[string]any)
	if !ok {
		t.Fatalf("expected org.service.router in servers: %v", servers)
	}
	if routerMap["command"] != "project-router" {
		t.Errorf("expected project-router to win precedence in json output, got: %v", routerMap["command"])
	}
}

// Interaction 6: Custom Output Path + Secret Redaction + Dry-Run
func TestTier3_Interaction_CustomOutput_SecretRedaction_DryRun(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "auth-srv": {
      "command": "auth-cli",
      "args": ["postgres://dbadmin:mypassword@localhost:5432/appdb"]
    }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	customOutput := "config/nested/team.toml"
	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--output", customOutput, "--dry-run")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	if !strings.Contains(stdout, "[DRY-RUN]") {
		t.Errorf("expected [DRY-RUN] in stdout: %s", stdout)
	}
	if !strings.Contains(stdout, "${DATABASE_URL}") {
		t.Errorf("expected ${DATABASE_URL} preview in dry-run stdout: %s", stdout)
	}

	// Neither parent dir nor file should be created
	if _, err := os.Stat(filepath.Join(projectPath, "config")); !os.IsNotExist(err) {
		t.Errorf("directory 'config' should not be created during dry-run")
	}
}

// Interaction 7: Nested Auth + Multiple Secret Types + Project & Global Merge
func TestTier3_Interaction_NestedAuth_MultipleSecretTypes_Merge(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Project Cursor config with complex nested auth
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	cursorJSON := `{
  "mcpServers": {
    "secure-gateway": {
      "type": "sse",
      "url": "https://gateway.example.com/sse",
      "auth": {
        "clientId": "corp-client-id",
        "clientSecret": "super-oauth-secret-key-12345678",
        "tokens": {
          "refresh": "refresh-token-xyz-987654321"
        }
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(cursorJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Global Codex config with database url and api key in env
	codexDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexTOML := `
[mcp_servers.global-storage]
command = "storage-mgr"
args = ["postgres://stuser:stpass@storage.net:5432/store"]
env.KEY = "sk-proj-storage-secret-key-12345"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexTOML), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.MCPServers) != 2 {
		t.Fatalf("expected 2 merged servers, got: %d", len(m.MCPServers))
	}

	// Verify all secrets redacted
	gw := m.MCPServers["secure-gateway"]
	authMap := gw.Auth.(map[string]any)
	if strings.Contains(authMap["clientSecret"].(string), "super-oauth") {
		t.Errorf("clientSecret leaked: %v", authMap["clientSecret"])
	}
	tokensMap := authMap["tokens"].(map[string]any)
	if strings.Contains(tokensMap["refresh"].(string), "refresh-token") {
		t.Errorf("refreshToken leaked: %v", tokensMap["refresh"])
	}

	st := m.MCPServers["global-storage"]
	if strings.Contains(st.Args[0], "stpass@") {
		t.Errorf("database password leaked: %s", st.Args[0])
	}
	if strings.Contains(st.Env["KEY"], "storage-secret") {
		t.Errorf("openai key leaked in env: %s", st.Env["KEY"])
	}
}
