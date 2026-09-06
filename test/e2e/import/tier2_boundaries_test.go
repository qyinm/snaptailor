package import_e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ============================================================================
// Category 1: Empty Files & Zero-Length Configs
// ============================================================================

func TestTier2_Boundary_EmptyFiles_EmptyMcpJson(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// 0-byte .mcp.json alongside a valid cursor config
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(`{"mcpServers":{"fallback":{"command":"ls"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "fallback") && !strings.Contains(stdout, "Successfully") {
		t.Errorf("expected successful import of fallback source, got stdout: %s", stdout)
	}
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, ok := m.MCPServers["fallback"]; !ok {
		t.Errorf("expected fallback server in manifest")
	}
}

func TestTier2_Boundary_EmptyFiles_EmptyCodexToml(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// 0-byte codex config alongside valid .mcp.json
	codexDir := filepath.Join(projectPath, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"tool":{"command":"node"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}
	assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
}

func TestTier2_Boundary_EmptyFiles_WhitespaceOnlyJson(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte("   \n\t  \n "), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(`{"mcpServers":{"global-tool":{"command":"echo"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, ok := m.MCPServers["global-tool"]; !ok {
		t.Errorf("expected global-tool to be imported despite whitespace .mcp.json")
	}
}

func TestTier2_Boundary_EmptyFiles_EmptyMcpServersObject(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(`{"mcpServers":{"active":{"command":"echo"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, ok := m.MCPServers["active"]; !ok {
		t.Errorf("expected active server in manifest")
	}
}

func TestTier2_Boundary_EmptyFiles_EmptySkillsDirectory(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Create empty skills directory
	skillsDir := filepath.Join(projectPath, ".cursor", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"s":{"command":"echo"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.Skills) != 0 {
		t.Errorf("expected 0 skills for empty skills directory, got: %d", len(m.Skills))
	}
}

func TestTier2_Boundary_EmptyFiles_SkillDirWithEmptySkillMD(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	skillDir := filepath.Join(projectPath, ".cursor", "skills", "empty-md-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	// 0-byte SKILL.md
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.Skills) != 1 || m.Skills[0].Name != "empty-md-skill" {
		t.Errorf("expected empty-md-skill to be recognized as valid skill: %+v", m.Skills)
	}
}

// ============================================================================
// Category 2: Malformed JSON / TOML & Syntax Errors
// ============================================================================

func TestTier2_Boundary_Malformed_SyntaxErrorInJsonWithValidFallback(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Broken JSON in .mcp.json
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers": { "unclosed": {`), 0644); err != nil {
		t.Fatal(err)
	}

	// Valid JSON in .cursor/mcp.json
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(`{"mcpServers":{"valid-srv":{"command":"node"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import should succeed using remaining valid sources: %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Warnings encountered") && !strings.Contains(stdout, "valid-srv") {
		t.Errorf("expected warnings or valid-srv in output, got stdout: %s", stdout)
	}
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, exists := m.MCPServers["valid-srv"]; !exists {
		t.Errorf("expected valid-srv in manifest")
	}
}

func TestTier2_Boundary_Malformed_SyntaxErrorInTomlWithValidFallback(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Broken TOML in .codex/config.toml
	codexDir := filepath.Join(projectPath, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`[mcp_servers.broken\ncommand = `), 0644); err != nil {
		t.Fatal(err)
	}

	// Valid .mcp.json
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"good-tool":{"command":"python"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, ok := m.MCPServers["good-tool"]; !ok {
		t.Errorf("expected good-tool in manifest")
	}
}

func TestTier2_Boundary_Malformed_AllCandidatesMalformed(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`invalid json non-object`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code == 0 {
		t.Fatalf("expected failure when all candidate sources are malformed")
	}
	if !strings.Contains(stderr, "failed to parse") && !strings.Contains(stderr, "all discovered candidate sources failed") {
		t.Errorf("expected parse error message in stderr, got: %s", stderr)
	}
}

func TestTier2_Boundary_Malformed_McpServersNotAnObject(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// mcpServers is a string rather than a map
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers": "not a map"}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code == 0 {
		t.Fatalf("expected non-zero exit code when mcpServers is not an object")
	}
	if !strings.Contains(stderr, "failed to parse") && !strings.Contains(stderr, "unmarshal") {
		t.Errorf("expected unmarshal error in stderr: %s", stderr)
	}
}

func TestTier2_Boundary_Malformed_InvalidArgsTypeInServer(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// args is an object instead of string array
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "bad-args": {
      "command": "node",
      "args": { "nested": "object" }
    }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code == 0 {
		t.Fatalf("expected non-zero exit code when args has invalid type")
	}
	if !strings.Contains(stderr, "unmarshal") && !strings.Contains(stderr, "failed to parse") {
		t.Errorf("expected unmarshal error in stderr: %s", stderr)
	}
}

// ============================================================================
// Category 3: Symlinks Escaping Project Root
// ============================================================================

func TestTier2_Boundary_Symlinks_DestSkillIsSymlinkOutside(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)
	outsideDir := t.TempDir()

	// Source skill
	skillDir := filepath.Join(projectPath, ".cursor", "skills", "target-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Target"), 0644); err != nil {
		t.Fatal(err)
	}

	// Destination .gandalf/skills/target-skill is a symlink pointing outside
	destParent := filepath.Join(projectPath, ".gandalf", "skills")
	if err := os.MkdirAll(destParent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(destParent, "target-skill")); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code == 0 {
		t.Fatalf("expected failure when destination skill is a symlink")
	}
	if !strings.Contains(stderr, "symlink") {
		t.Errorf("expected symlink security violation, got: %s", stderr)
	}
}

func TestTier2_Boundary_Symlinks_ParentDirIsSymlink(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)
	outsideDir := t.TempDir()

	// Source skill
	skillDir := filepath.Join(projectPath, ".cursor", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Make .gandalf a symlink pointing outside
	if err := os.Symlink(outsideDir, filepath.Join(projectPath, ".gandalf")); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code == 0 {
		t.Fatalf("expected failure when parent .gandalf directory is a symlink")
	}
	if !strings.Contains(stderr, "symlink") {
		t.Errorf("expected symlink security error in stderr: %s", stderr)
	}
}

func TestTier2_Boundary_Symlinks_OutputFileIsSymlink(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)
	outsideDir := t.TempDir()
	outsideTarget := filepath.Join(outsideDir, "stolen.toml")

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"s":{"command":"ls"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Make gandalf.toml a symlink pointing to outsideTarget
	if err := os.Symlink(outsideTarget, filepath.Join(projectPath, "gandalf.toml")); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only", "--force")
	if code == 0 {
		t.Fatalf("expected failure when output file is a symlink")
	}
	if !strings.Contains(stderr, "symlink") {
		t.Errorf("expected symlink error in stderr: %s", stderr)
	}
}

func TestTier2_Boundary_Symlinks_SourceSkillIsSymlink(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)
	outsideSkillDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideSkillDir, "SKILL.md"), []byte("# External Secret Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Source skills folder contains a symlink to outside
	skillsDir := filepath.Join(projectPath, ".cursor", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideSkillDir, filepath.Join(skillsDir, "external-symlink")); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	// External symlink must NOT be mirrored into .gandalf/skills/
	dest := filepath.Join(projectPath, ".gandalf", "skills", "external-symlink")
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Errorf("security violation: source symlinked skill directory was mirrored to destination")
	}
}

func TestTier2_Boundary_Symlinks_ParentDirOfOutputFileIsSymlink(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)
	outsideDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"s":{"command":"ls"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink dir pointing outside
	linkDir := filepath.Join(projectPath, "sym_dir")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--output", "sym_dir/manifest.toml")
	if code == 0 {
		t.Fatalf("expected error when output parent directory is a symlink")
	}
	if !strings.Contains(stderr, "security violation") && !strings.Contains(stderr, "symlink") {
		t.Errorf("expected security violation in stderr: %s", stderr)
	}
}

// ============================================================================
// Category 4: Path Traversal Attempts
// ============================================================================

func TestTier2_Boundary_PathTraversal_OutputFileEscapesRoot(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"s":{"command":"ls"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--output", "../escaped.toml")
	if code == 0 {
		t.Fatalf("expected failure for output escaping project root")
	}
	if !strings.Contains(stderr, "escapes project root") && !strings.Contains(stderr, "security violation") {
		t.Errorf("expected security violation in stderr: %s", stderr)
	}
}

func TestTier2_Boundary_PathTraversal_DeepTraversalOutputFile(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{"mcpServers":{"s":{"command":"ls"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--output", "nested/sub/../../../../etc/malicious.toml")
	if code == 0 {
		t.Fatalf("expected failure for deep path traversal output")
	}
	if !strings.Contains(stderr, "escapes project root") && !strings.Contains(stderr, "security violation") {
		t.Errorf("expected security violation in stderr: %s", stderr)
	}
}

func TestTier2_Boundary_PathTraversal_FromNonExistentTarget(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--from", "non_existent_dir/mcp.json")
	if code == 0 {
		t.Fatalf("expected error for non-existent --from path")
	}
	if !strings.Contains(stderr, "does not exist") {
		t.Errorf("expected 'does not exist' in stderr: %s", stderr)
	}
}

func TestTier2_Boundary_PathTraversal_EnvFileWithParentTraversal(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	// envFile attempting path traversal outside
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(`{
  "mcpServers": {
    "unsafe-envfile": {
      "command": "node",
      "envFile": "../../etc/shadow"
    }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["unsafe-envfile"]
	// Escaped envFile must be sanitized to empty
	if srv.EnvFile != "" {
		t.Errorf("expected unsafe envFile to be stripped, got: %s", srv.EnvFile)
	}
}

func TestTier2_Boundary_PathTraversal_AbsoluteEnvFileStripped(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "abs-env": {
      "command": "python",
      "envFile": "/etc/passwd"
    }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["abs-env"]
	if srv.EnvFile != "" {
		t.Errorf("expected absolute envFile to be stripped, got: %s", srv.EnvFile)
	}
}

// ============================================================================
// Category 5: Missing Variables & Edge Cases in Env Template
// ============================================================================

func TestTier2_Boundary_MissingEnvs_UnreferencedVarPreserved(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Server referencing existing ${MY_CUSTOM_VAR}
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "custom-env-srv": {
      "command": "echo",
      "args": ["${MY_CUSTOM_VAR}"]
    }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["custom-env-srv"]
	found := false
	for _, req := range srv.RequiredEnv {
		if req == "MY_CUSTOM_VAR" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MY_CUSTOM_VAR in required_env: %v", srv.RequiredEnv)
	}
}

func TestTier2_Boundary_MissingEnvs_MultipleSecretsInSingleArg(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Multiple secrets in one arg string
	mcpJSON := `{
  "mcpServers": {
    "multi-secret": {
      "command": "run",
      "args": [
        "--db=postgres://user:pass@host:5432/db --key=sk-ant-api03-abcdefghijklmnop"
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
	srv := m.MCPServers["multi-secret"]
	arg := srv.Args[0]
	if strings.Contains(arg, "pass@") || strings.Contains(arg, "abcdefghijklmnop") {
		t.Errorf("raw secret leaked in arg: %s", arg)
	}
	if !strings.Contains(arg, "${DATABASE_URL}") {
		t.Errorf("expected ${DATABASE_URL} in arg, got: %s", arg)
	}
	if !strings.Contains(arg, "${ANTHROPIC_API_KEY}") {
		t.Errorf("expected ${ANTHROPIC_API_KEY} in arg, got: %s", arg)
	}
}

func TestTier2_Boundary_MissingEnvs_ConflictingVarNamesAcrossServers(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "service-a": {
      "command": "node",
      "env": { "API_KEY": "sk-ant-api03-key-one-123456789012" }
    },
    "service-b": {
      "command": "python",
      "env": { "API_KEY": "sk-ant-api03-key-two-987654321098" }
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
	srvA := m.MCPServers["service-a"]
	srvB := m.MCPServers["service-b"]
	if srvA.Env["API_KEY"] == srvB.Env["API_KEY"] {
		// If collision occurred, second server should be scoped
		t.Logf("Note: Both servers mapped to %s", srvA.Env["API_KEY"])
	}
	for _, srv := range []manifest.MCPServerDef{srvA, srvB} {
		for _, v := range srv.Env {
			if strings.Contains(v, "key-one") || strings.Contains(v, "key-two") {
				t.Fatalf("raw secret exposed in env: %s", v)
			}
		}
	}
}

func TestTier2_Boundary_MissingEnvs_DefaultFallbackSyntax(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "fallback-tool": {
      "command": "server",
      "args": ["${PORT:-8080}"]
    }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s", code, stderr)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	srv := m.MCPServers["fallback-tool"]
	foundPort := false
	for _, req := range srv.RequiredEnv {
		if req == "PORT" {
			foundPort = true
			break
		}
	}
	if !foundPort {
		t.Errorf("expected PORT extracted into required_env from ${PORT:-8080}, got: %v", srv.RequiredEnv)
	}
}

func TestTier2_Boundary_MissingEnvs_SanitizedPlaceholdersNeverExposeRealSecret(t *testing.T) {
	t.Parallel()
	envTemplate := make(map[string]string)
	srv := manifest.MCPServerDef{
		Command: "connect postgres://super_secret_user:super_secret_pw@db.net:5432/secrets",
	}

	importer.RedactAndTemplatizeServer("database", &srv, envTemplate)

	for k, v := range envTemplate {
		if strings.Contains(v, "super_secret") {
			t.Fatalf("secret leaked in placeholder %s: %s", k, v)
		}
	}
}

// ============================================================================
// Category 6: Unusual Characters in Server Names, Keys, and Values
// ============================================================================

func TestTier2_Boundary_Unusual_DottedServerName(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "com.corp.internal.service": {
      "command": "run-service",
      "args": ["--port=80"]
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

	manifestBytes, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(manifestBytes)
	if !strings.Contains(content, `[mcp_servers."com.corp.internal.service"]`) {
		t.Errorf("expected dotted server name to be quoted in TOML header: %s", content)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, ok := m.MCPServers["com.corp.internal.service"]; !ok {
		t.Errorf("expected com.corp.internal.service in parsed manifest")
	}
}

func TestTier2_Boundary_Unusual_ServerNameWithSpacesAndHyphens(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "my cool - server": {
      "command": "node",
      "args": ["main.js"]
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

	manifestBytes, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(manifestBytes)
	if !strings.Contains(content, `[mcp_servers."my cool - server"]`) {
		t.Errorf("expected space-containing server name to be quoted: %s", content)
	}

	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, ok := m.MCPServers["my cool - server"]; !ok {
		t.Errorf("expected 'my cool - server' in parsed manifest")
	}
}

func TestTier2_Boundary_Unusual_SpecialCharsInHeaders(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "special-headers": {
      "type": "sse",
      "url": "https://example.com/sse",
      "headers": {
        "X-Dotted.Custom.Header": "value1",
        "Header With Space": "value2"
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
	srv := m.MCPServers["special-headers"]
	if srv.Headers["X-Dotted.Custom.Header"] != "value1" {
		t.Errorf("expected dotted header preserved, got: %v", srv.Headers)
	}
	if srv.Headers["Header With Space"] != "value2" {
		t.Errorf("expected space header preserved, got: %v", srv.Headers)
	}
}

func TestTier2_Boundary_Unusual_UnicodeInServerDescription(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "intl-tool": {
      "command": "intl",
      "description": "日本語の説明と絵文字 🚀 and accented café"
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
	srv := m.MCPServers["intl-tool"]
	if !strings.Contains(srv.Description, "日本語") || !strings.Contains(srv.Description, "🚀") {
		t.Errorf("unicode description corrupted: %s", srv.Description)
	}
}

func TestTier2_Boundary_Unusual_EscapedQuotesAndBackslashesInCommands(t *testing.T) {
	t.Parallel()
	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "windows-escape-test",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		MCPServers: map[string]manifest.MCPServerDef{
			"win-srv": {
				Command:     `C:\Program Files (x86)\App\server.exe`,
				Args:        []string{`--config="C:\data\cfg.json"`, `quoted "value"`},
				Description: `Quotes "here" and backslashes \there\`,
			},
		},
	}

	formatted := importer.FormatManifestTOML(m)
	parsed, err := manifest.Parse(formatted, &manifest.ParseOptions{NoInterpolate: true})
	if err != nil {
		t.Fatalf("failed to parse TOML with escapes: %v\nFormatted:\n%s", err, formatted)
	}

	srv := parsed.Manifest.MCPServers["win-srv"]
	if srv.Command != `C:\Program Files (x86)\App\server.exe` {
		t.Errorf("command roundtrip mismatch: %s", srv.Command)
	}
	if len(srv.Args) != 2 || srv.Args[0] != `--config="C:\data\cfg.json"` {
		t.Errorf("args roundtrip mismatch: %v", srv.Args)
	}
}

func TestTier2_Boundary_Unusual_AtSignAndHyphensInServerName(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	mcpJSON := `{
  "mcpServers": {
    "@scope/package-tool": {
      "command": "npx",
      "args": ["@scope/package-tool"]
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
	if _, exists := m.MCPServers["@scope/package-tool"]; !exists {
		t.Errorf("expected @scope/package-tool in manifest")
	}
}
