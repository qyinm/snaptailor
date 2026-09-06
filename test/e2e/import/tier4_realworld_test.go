package import_e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// Tier 4: Real-World Scenarios
// ============================================================================

// Scenario 1: Polyglot monorepo migration with Claude Code + Cursor + Codex
// simultaneously configured, conflicting server names, and team skills.
// Asserts manifest.Validate returns 0 errors and gandalf check --project-only passes with InSync = true.
func TestTier4_RealWorld_PolyglotMonorepoMigration(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// 1. Claude Code: .mcp.json
	claudeMCP := `{
  "mcpServers": {
    "db-service": {
      "command": "npx",
      "args": ["-y", "@mcp/postgres", "postgres://admin:prod_password_123@db.prod.internal:5432/main_db"]
    },
    "git-tools": {
      "command": "python",
      "args": ["-m", "git_mcp_server"],
      "env": {
        "GIT_TOKEN": "ghp_0123456789abcdefghijklmnopqrstuvwxyz"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(claudeMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Claude Code skills: .claude/skills/code-review/SKILL.md
	claudeSkillDir := filepath.Join(projectPath, ".claude", "skills", "code-review")
	if err := os.MkdirAll(claudeSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeSkillDir, "SKILL.md"), []byte("# Team Code Review Skill\nReview checklist."), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Cursor: .cursor/mcp.json
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	cursorMCP := `{
  "mcpServers": {
    "cursor-browser": {
      "type": "sse",
      "url": "https://browser.mcp.internal/sse",
      "headers": {
        "Authorization": "Bearer super-jwt-token-abcdef123456"
      }
    },
    "db-service": {
      "command": "npx",
      "args": ["-y", "@mcp/postgres", "postgres://admin:prod_password_123@db.prod.internal:5432/main_db"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(cursorMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Cursor skills: .cursor/skills/unit-test-gen/SKILL.md
	cursorSkillDir := filepath.Join(projectPath, ".cursor", "skills", "unit-test-gen")
	if err := os.MkdirAll(cursorSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorSkillDir, "SKILL.md"), []byte("# Unit Test Generation\nTest guidelines."), 0644); err != nil {
		t.Fatal(err)
	}

	// 5. Codex: .codex/config.toml
	codexDir := filepath.Join(projectPath, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	codexTOML := `
[mcp_servers.codex-evaluator]
command = "pytest"
args = ["-q", "--benchmark"]
env.EVAL_ENV = "test"
`
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexTOML), 0644); err != nil {
		t.Fatal(err)
	}

	// 6. Codex skills: .codex/skills/benchmark/SKILL.md
	codexSkillDir := filepath.Join(projectPath, ".codex", "skills", "benchmark")
	if err := os.MkdirAll(codexSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexSkillDir, "SKILL.md"), []byte("# Benchmark Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run gandalf export
	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("gandalf export failed with code %d.\nStdout: %s\nStderr: %s", code, stdout, stderr)
	}

	// Verification 1: Manifest generated and passes manifest.Validate with 0 errors
	manifestPath := filepath.Join(projectPath, "gandalf.toml")
	m := assertManifestValid(t, projectPath, manifestPath)

	// Verification 2: Check server unification and secret templatization
	if len(m.MCPServers) != 4 {
		t.Fatalf("expected 4 unified servers (db-service, git-tools, cursor-browser, codex-evaluator), got %d", len(m.MCPServers))
	}
	for _, expectedName := range []string{"db-service", "git-tools", "cursor-browser", "codex-evaluator"} {
		if _, exists := m.MCPServers[expectedName]; !exists {
			t.Errorf("expected server '%s' in manifest", expectedName)
		}
	}

	// Verification 3: Skills mirrored to .gandalf/skills/
	if len(m.Skills) != 3 {
		t.Fatalf("expected 3 mirrored skills, got %d: %+v", len(m.Skills), m.Skills)
	}
	for _, expectedSkill := range []string{"code-review", "unit-test-gen", "benchmark"} {
		destMD := filepath.Join(projectPath, ".gandalf", "skills", expectedSkill, "SKILL.md")
		if !fileExists(destMD) {
			t.Errorf("expected mirrored skill file '%s' to exist", destMD)
		}
	}

	// Verification 4: Align repository agent files with imported canonical manifest
	syncProjectAgentConfigs(t, projectPath, m)

	// Verification 5: Crucial Acceptance Criteria: gandalf check --project-only passes with InSync = true
	assertCheckInSync(t, projectPath)
}

// Scenario 2: Multi-developer enterprise setup where personal global configs (~/.claude.json, ~/.cursor/mcp.json)
// are merged with repository project configs, secrets are templatized, manifest validates with 0 errors,
// and gandalf check --project-only passes with InSync = true.
func TestTier4_RealWorld_EnterpriseMultiDeveloperSetup(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// 1. Developer global configuration (~/.claude.json)
	devClaudeJSON := `{
  "mcpServers": {
    "personal-linter": {
      "command": "eslint-mcp",
      "args": ["--cache"]
    },
    "shared-db": {
      "command": "psql-mcp",
      "args": ["postgres://devuser:devpass@localhost:5432/localdb"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(devClaudeJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Developer global Cursor configuration (~/.cursor/mcp.json)
	globalCursorDir := filepath.Join(homeDir, ".cursor")
	if err := os.MkdirAll(globalCursorDir, 0755); err != nil {
		t.Fatal(err)
	}
	devCursorJSON := `{
  "mcpServers": {
    "personal-formatter": {
      "command": "prettier-mcp"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(globalCursorDir, "mcp.json"), []byte(devCursorJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Project repository configuration (.mcp.json)
	projMCP := `{
  "mcpServers": {
    "team-service": {
      "command": "npx",
      "args": ["-y", "@corp/team-server", "https://api.corp.internal/mcp"],
      "env": {
        "CORP_API_TOKEN": "sk-ant-api03-abcdef1234567890abcdef123456"
      }
    },
    "shared-db": {
      "command": "psql-mcp",
      "args": ["postgres://teamuser:teampass@db.corp.internal:5432/teamdb"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(projMCP), 0644); err != nil {
		t.Fatal(err)
	}

	// Run import across project and global
	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir)
	if code != 0 {
		t.Fatalf("import failed: %d, stderr: %s, stdout: %s", code, stderr, stdout)
	}

	// Verify generated manifest
	manifestPath := filepath.Join(projectPath, "gandalf.toml")
	m := assertManifestValid(t, projectPath, manifestPath)

	// Verify project takes precedence over global for shared-db
	sharedDB := m.MCPServers["shared-db"]
	if strings.Contains(sharedDB.Args[0], "devuser") || strings.Contains(sharedDB.Args[0], "localdb") {
		t.Errorf("expected project shared-db to override developer local shared-db, got: %v", sharedDB.Args)
	}

	// Verify all secrets are masked
	for _, srv := range m.MCPServers {
		for _, arg := range srv.Args {
			if strings.Contains(arg, "teampass") || strings.Contains(arg, "devpass") {
				t.Errorf("secret leaked into server args: %s", arg)
			}
		}
		for k, v := range srv.Env {
			if strings.Contains(v, "abcdef1234567890") {
				t.Errorf("secret leaked into server env %s: %s", k, v)
			}
		}
	}

	// Align repository agent files with the imported canonical manifest
	syncProjectAgentConfigs(t, projectPath, m)

	// Verify gandalf check --project-only passes with InSync = true
	assertCheckInSync(t, projectPath)
}

// Scenario 3: CI Fresh Clone Pipeline gate
// In CI, automated pipeline runs gandalf export --project-only, then immediately runs
// gandalf check --project-only --ci. Asserts exit code 0 and InSync = true.
func TestTier4_RealWorld_CIPipelineDriftGate(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Simulated fresh clone with project MCP and skills
	mcpJSON := `{
  "mcpServers": {
    "ci-service": {
      "command": "ci-tool",
      "args": ["--port=9090"],
      "env": {
        "BUILD_SECRET": "ghp_0123456789abcdefghijklmnopqrstuvwxyz"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(projectPath, ".claude", "skills", "ci-builder")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# CI Builder Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	// Step 1: CI runs gandalf export --project-only
	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("CI import step failed: %d\nStdout: %s\nStderr: %s", code, stdout, stderr)
	}

	// Step 2: Verify manifest validation returns 0 errors
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))

	// Step 3: Align repository agent files with the imported canonical manifest
	syncProjectAgentConfigs(t, projectPath, m)

	// Step 4: CI runs gandalf check --project-only --ci
	assertCheckInSync(t, projectPath)
}

// Scenario 4: Custom legacy tooling setup imported via --from <dir>
// Asserts manifest validation passes with 0 errors and gandalf check --project-only passes with InSync = true.
func TestTier4_RealWorld_LegacyAgentMigrationViaFromDir(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// Legacy directory
	legacyDir := filepath.Join(projectPath, "legacy-ai-setup")
	if err := os.MkdirAll(filepath.Join(legacyDir, "skills", "legacy-workflow"), 0755); err != nil {
		t.Fatal(err)
	}
	legacyMCP := `{
  "mcpServers": {
    "legacy-tool": {
      "command": "node",
      "args": ["legacy.js", "postgres://user:legacy_pass@host:5432/db"]
    }
  }
}`
	if err := os.WriteFile(filepath.Join(legacyDir, "mcp.json"), []byte(legacyMCP), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "skills", "legacy-workflow", "SKILL.md"), []byte("# Legacy Workflow"), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--from", legacyDir, "--project-only")
	if code != 0 {
		t.Fatalf("import from legacyDir failed: %d\nStdout: %s\nStderr: %s", code, stdout, stderr)
	}

	// Manifest validation
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if _, ok := m.MCPServers["legacy-tool"]; !ok {
		t.Errorf("expected legacy-tool in manifest")
	}

	// Skills mirrored
	if !fileExists(filepath.Join(projectPath, ".gandalf", "skills", "legacy-workflow", "SKILL.md")) {
		t.Errorf("expected legacy-workflow skill mirrored to .gandalf/skills/")
	}

	// Align repository agent files with imported canonical manifest
	syncProjectAgentConfigs(t, projectPath, m)

	// Check passes with InSync = true
	assertCheckInSync(t, projectPath)
}

// Scenario 5: Incremental Re-Import & Idempotency
// Repository already has gandalf.toml. A new agent (Cursor) is added with new servers and skills.
// gandalf export --project-only --force is run. Asserts manifest updates safely, subsequent run
// is completely idempotent, and gandalf check --project-only passes with InSync = true.
func TestTier4_RealWorld_IdempotentReImportAfterUpdate(t *testing.T) {
	t.Parallel()
	projectPath, homeDir, _ := makeSandbox(t)

	// 1. Initial setup with Claude Code
	if err := os.WriteFile(filepath.Join(projectPath, ".mcp.json"), []byte(`{
  "mcpServers": {
    "initial-service": { "command": "init-cmd" }
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only")
	if code != 0 {
		t.Fatalf("initial import failed: %d, stderr: %s", code, stderr)
	}
	assertCheckInSync(t, projectPath)

	// 2. New engineer introduces Cursor setup with server and skill
	cursorDir := filepath.Join(projectPath, ".cursor")
	if err := os.MkdirAll(filepath.Join(cursorDir, "skills", "doc-gen"), 0755); err != nil {
		t.Fatal(err)
	}
	cursorMCP := `{
  "mcpServers": {
    "cursor-doc-service": { "command": "doc-cmd" }
  }
}`
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte(cursorMCP), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "skills", "doc-gen", "SKILL.md"), []byte("# Doc Gen"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Re-import with --force
	_, stderr, code = runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only", "--force")
	if code != 0 {
		t.Fatalf("re-import with force failed: %d, stderr: %s", code, stderr)
	}

	manifestBytes1, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatal(err)
	}

	// 4. Verify manifest has both initial and new services
	m := assertManifestValid(t, projectPath, filepath.Join(projectPath, "gandalf.toml"))
	if len(m.MCPServers) != 2 {
		t.Fatalf("expected 2 servers after re-import, got: %d", len(m.MCPServers))
	}
	if _, ok := m.MCPServers["initial-service"]; !ok {
		t.Errorf("initial-service missing after re-import")
	}
	if _, ok := m.MCPServers["cursor-doc-service"]; !ok {
		t.Errorf("cursor-doc-service missing after re-import")
	}

	// 5. Run import a third time to verify strict idempotency
	_, stderr, code = runCLI(t, "export", "--project", projectPath, "--home", homeDir, "--project-only", "--force")
	if code != 0 {
		t.Fatalf("third import failed: %d, stderr: %s", code, stderr)
	}
	manifestBytes2, err := os.ReadFile(filepath.Join(projectPath, "gandalf.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(manifestBytes1) != string(manifestBytes2) {
		t.Errorf("re-import is not idempotent! Diff:\nRun 1:\n%s\nRun 2:\n%s", string(manifestBytes1), string(manifestBytes2))
	}

	// 6. Align repository agent files with the updated manifest
	syncProjectAgentConfigs(t, projectPath, m)

	// 7. Final verification: Check passes with InSync = true
	assertCheckInSync(t, projectPath)
}
