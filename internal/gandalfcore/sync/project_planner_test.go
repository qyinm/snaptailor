package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/restore"
	_ "github.com/qyinm/gandalf/internal/gandalfcore/scan/plugins"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

func TestCreateProjectSyncPlan_CreatesMissingConfigsAndCheckPasses(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := filepath.Join(tempDir, "project")
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "project-apply",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCodex, types.AgentCursor},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-echo": {
				Command: "echo",
				Args:    []string{"${APP_ENV}"},
			},
		},
		EnvTemplate: map[string]string{
			"APP_ENV": "production",
		},
	}

	plan, err := CreateProjectSyncPlan(m, projectRoot, homeDir)
	if err != nil {
		t.Fatalf("CreateProjectSyncPlan: %v", err)
	}
	if len(plan.Items) != 3 {
		t.Fatalf("expected 3 project config items, got %d", len(plan.Items))
	}

	for _, item := range plan.Items {
		if !strings.HasPrefix(item.TargetFile, projectRoot) {
			t.Errorf("project plan must target project root, got %s", item.TargetFile)
		}
		if strings.Contains(item.Content, "production") {
			t.Errorf("project plan must keep ${APP_ENV} uninterpolated, got:\n%s", item.Content)
		}
		if !strings.Contains(item.Content, "${APP_ENV}") {
			t.Errorf("expected ${APP_ENV} template in project content:\n%s", item.Content)
		}
	}

	roots := &pathconfinement.Roots{HomeDir: homeDir, ProjectPath: projectRoot}
	result, err := ApplySyncPlan(plan, roots, "")
	if err != nil {
		t.Fatalf("ApplySyncPlan: %v", err)
	}
	if !result.Success {
		t.Fatalf("apply failed: %v", result.Errors)
	}

	if _, err := os.Stat(filepath.Join(homeDir, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("project-only apply must not write user-home Claude settings")
	}

	report, err := DetectProjectDrift(m, projectRoot)
	if err != nil {
		t.Fatalf("DetectProjectDrift: %v", err)
	}
	if !report.InSync {
		t.Fatalf("expected InSync after project apply, items: %+v", report.Items)
	}
}

func TestCreateSyncPlan_DoesNotCoverProjectMCPFiles(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := filepath.Join(tempDir, "project")
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "home-only",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-echo": {Command: "echo"},
		},
	}

	plan, err := CreateSyncPlan(m, projectRoot, homeDir, nil)
	if err != nil {
		t.Fatalf("CreateSyncPlan: %v", err)
	}

	roots := &pathconfinement.Roots{HomeDir: homeDir, ProjectPath: projectRoot}
	result, err := ApplySyncPlan(plan, roots, "")
	if err != nil {
		t.Fatalf("ApplySyncPlan: %v", err)
	}
	if !result.Success {
		t.Fatalf("home apply failed: %v", result.Errors)
	}

	if _, err := os.Stat(filepath.Join(projectRoot, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("default home apply must not create project .mcp.json")
	}

	report, err := DetectProjectDrift(m, projectRoot)
	if err != nil {
		t.Fatalf("DetectProjectDrift: %v", err)
	}
	if report.InSync {
		t.Fatal("project-only check must still report missing project MCP after home-only apply")
	}
}

func TestCreateProjectSyncPlan_RemovesUnmanagedProjectServers(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(`{
  "customKey": true,
  "mcpServers": {
    "personal-db": {"command": "sqlite3"}
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "reconcile",
		Agents:  []types.AgentID{types.AgentClaudeCode},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-echo": {Command: "echo"},
		},
	}

	plan, err := CreateProjectSyncPlan(m, tempDir, filepath.Join(tempDir, "home"))
	if err != nil {
		t.Fatalf("CreateProjectSyncPlan: %v", err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(plan.Items))
	}
	if strings.Contains(plan.Items[0].Content, "personal-db") {
		t.Errorf("project apply must drop unmanaged servers, got:\n%s", plan.Items[0].Content)
	}
	if !strings.Contains(plan.Items[0].Content, "team-echo") {
		t.Errorf("expected team-echo to be added, got:\n%s", plan.Items[0].Content)
	}
	if !strings.Contains(plan.Items[0].Content, "customKey") {
		t.Errorf("expected non-server keys to be preserved, got:\n%s", plan.Items[0].Content)
	}
}

func TestCreateProjectSyncPlan_EmptyServersIsNoop(t *testing.T) {
	tempDir := t.TempDir()
	m := &manifest.Manifest{
		Version:    "1.0",
		Name:       "empty",
		Agents:     []types.AgentID{types.AgentClaudeCode, types.AgentCursor, types.AgentCodex},
		MCPServers: map[string]manifest.MCPServerDef{},
	}

	plan, err := CreateProjectSyncPlan(m, tempDir, tempDir)
	if err != nil {
		t.Fatalf("CreateProjectSyncPlan: %v", err)
	}
	if len(plan.Items) != 0 {
		t.Fatalf("expected no project writes when no servers are declared, got %d", len(plan.Items))
	}
}

func TestApplyProjectSyncPlan_RevalidatesConcurrentJSONAndTOMLEdits(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := filepath.Join(tempDir, "project")
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(projectRoot, ".mcp.json"), []byte(`{
  "customKey": true,
  "mcpServers": {}
}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "config.toml"), []byte("model = \"gpt-5\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "stale-plan",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCodex},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-echo": {Command: "echo"},
		},
	}

	plan, err := CreateProjectSyncPlan(m, projectRoot, homeDir)
	if err != nil {
		t.Fatalf("CreateProjectSyncPlan: %v", err)
	}

	if err := os.WriteFile(filepath.Join(projectRoot, ".mcp.json"), []byte(`{
  "customKey": true,
  "theme": "dark",
  "mcpServers": {}
}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "config.toml"), []byte("model = \"gpt-5\"\napproval_policy = \"on-request\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	roots := &pathconfinement.Roots{HomeDir: homeDir, ProjectPath: projectRoot}
	result, err := ApplySyncPlan(plan, roots, "")
	if err != nil {
		t.Fatalf("ApplySyncPlan: %v", err)
	}
	if !result.Success {
		t.Fatalf("apply failed: %v", result.Errors)
	}

	mcp := readFile(t, filepath.Join(projectRoot, ".mcp.json"))
	if !strings.Contains(mcp, "theme") || !strings.Contains(mcp, "dark") {
		t.Fatalf("concurrent JSON setting was overwritten:\n%s", mcp)
	}
	if !strings.Contains(mcp, "team-echo") {
		t.Fatalf("expected team-echo after re-merge:\n%s", mcp)
	}
	codex := readFile(t, filepath.Join(projectRoot, ".codex", "config.toml"))
	if !strings.Contains(codex, "approval_policy") {
		t.Fatalf("concurrent TOML setting was overwritten:\n%s", codex)
	}
	if !strings.Contains(codex, "team-echo") {
		t.Fatalf("expected team-echo in Codex config:\n%s", codex)
	}
}

func TestApplyProjectSyncPlan_ExtraServersRoundTripJSONAndCodex(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := filepath.Join(tempDir, "project")
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(projectRoot, ".mcp.json"), []byte(`{
  "mcpServers": {
    "extra-json": {"command": "sqlite3"},
    "team-echo": {"command": "old"}
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "config.toml"), []byte(`
[mcp_servers.extra-toml]
command = "local"

[mcp_servers.team-echo]
command = "old"
`), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "extras",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCodex},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-echo": {Command: "echo"},
		},
	}

	before, err := DetectProjectDrift(m, projectRoot)
	if err != nil {
		t.Fatalf("DetectProjectDrift before: %v", err)
	}
	if before.InSync {
		t.Fatal("expected extras to fail project check before apply")
	}

	plan, err := CreateProjectSyncPlan(m, projectRoot, homeDir)
	if err != nil {
		t.Fatalf("CreateProjectSyncPlan: %v", err)
	}
	roots := &pathconfinement.Roots{HomeDir: homeDir, ProjectPath: projectRoot}
	result, err := ApplySyncPlan(plan, roots, "")
	if err != nil {
		t.Fatalf("ApplySyncPlan: %v", err)
	}
	if !result.Success {
		t.Fatalf("apply failed: %v", result.Errors)
	}

	mcp := readFile(t, filepath.Join(projectRoot, ".mcp.json"))
	if strings.Contains(mcp, "extra-json") {
		t.Fatalf("JSON extra server survived apply:\n%s", mcp)
	}
	codex := readFile(t, filepath.Join(projectRoot, ".codex", "config.toml"))
	if strings.Contains(codex, "extra-toml") {
		t.Fatalf("Codex extra server survived apply:\n%s", codex)
	}

	after, err := DetectProjectDrift(m, projectRoot)
	if err != nil {
		t.Fatalf("DetectProjectDrift after: %v", err)
	}
	if !after.InSync {
		t.Fatalf("expected InSync after extra-server reconcile, items: %+v", after.Items)
	}
}

func TestApplyProjectSyncPlan_SnapshotCapturesAndRestoresProjectFiles(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := filepath.Join(tempDir, "project")
	homeDir := filepath.Join(tempDir, "home")
	storeDir := filepath.Join(tempDir, "store")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".codex"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		t.Fatal(err)
	}

	originalMCP := "{\n  \"mcpServers\": {\n    \"legacy\": {\"command\": \"old-mcp\"}\n  }\n}\n"
	originalCursor := "{\n  \"mcpServers\": {\n    \"legacy\": {\"command\": \"old-cursor\"}\n  }\n}\n"
	originalCodex := "[mcp_servers.legacy]\ncommand = \"old-codex\"\n"
	if err := os.WriteFile(filepath.Join(projectRoot, ".mcp.json"), []byte(originalMCP), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".cursor", "mcp.json"), []byte(originalCursor), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".codex", "config.toml"), []byte(originalCodex), 0644); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Version: "1.0",
		Name:    "backup",
		Agents:  []types.AgentID{types.AgentClaudeCode, types.AgentCursor, types.AgentCodex},
		MCPServers: map[string]manifest.MCPServerDef{
			"team-echo": {Command: "echo"},
		},
	}

	plan, err := CreateProjectSyncPlan(m, projectRoot, homeDir)
	if err != nil {
		t.Fatalf("CreateProjectSyncPlan: %v", err)
	}
	roots := &pathconfinement.Roots{HomeDir: homeDir, ProjectPath: projectRoot}
	result, err := ApplySyncPlan(plan, roots, storeDir)
	if err != nil {
		t.Fatalf("ApplySyncPlan: %v", err)
	}
	if !result.Success {
		t.Fatalf("apply failed: %v", result.Errors)
	}
	if result.BackupSnapshot == "" {
		t.Fatal("expected a reported rollback snapshot for project apply")
	}

	snap, err := store.ReadSnapshot(storeDir, result.BackupSnapshot, nil)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	want := map[string]string{
		".mcp.json":                      originalMCP,
		filepath.ToSlash(filepath.Join(".cursor", "mcp.json")): originalCursor,
		filepath.ToSlash(filepath.Join(".codex", "config.toml")): originalCodex,
	}
	found := map[string]bool{}
	for _, entry := range snap.Content {
		path := filepath.ToSlash(entry.SourcePath)
		original, ok := want[path]
		if !ok || entry.CaptureStatus != "captured" {
			continue
		}
		text, err := store.ReadSnapshotContent(storeDir, result.BackupSnapshot, entry, nil)
		if err != nil {
			t.Fatalf("ReadSnapshotContent %s: %v", path, err)
		}
		if text != original {
			t.Fatalf("snapshot %s = %q, want original", path, text)
		}
		found[path] = true
	}
	for path := range want {
		if !found[path] {
			t.Fatalf("snapshot missing captured content for %s", path)
		}
	}

	scope := types.ScopeProject
	restorePlan, err := restore.BuildRestorePlan(&types.RestoreOptions{
		SourceSnapshot: result.BackupSnapshot,
		ProjectPath:    projectRoot,
		HomeDir:        homeDir,
		StoreDir:       storeDir,
		DryRun:         true,
		Scope:          &scope,
	})
	if err != nil {
		t.Fatalf("BuildRestorePlan: %v", err)
	}
	planJSON, err := json.Marshal(restorePlan)
	if err != nil {
		t.Fatal(err)
	}
	parsed := restore.ParseDryRunOutput(string(planJSON))
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse restore plan: %#v", parsed.Errors)
	}
	summary := restore.ApplyRestoreItems(parsed.Items, restore.CreateDefaultApplyExecutor(), &types.ApplyOptions{
		FailFast:    true,
		HomeDir:     &homeDir,
		ProjectPath: &projectRoot,
	})
	if summary.Failed != 0 {
		t.Fatalf("restore apply failed: %#v", summary.Failures)
	}

	assertRestoredProjectServer(t, readFile(t, filepath.Join(projectRoot, ".mcp.json")), "old-mcp", "echo")
	assertRestoredProjectServer(t, readFile(t, filepath.Join(projectRoot, ".cursor", "mcp.json")), "old-cursor", "echo")
	assertRestoredProjectServer(t, readFile(t, filepath.Join(projectRoot, ".codex", "config.toml")), "old-codex", "echo")
}

func assertRestoredProjectServer(t *testing.T, got, wantCommand, appliedCommand string) {
	t.Helper()
	if !strings.Contains(got, wantCommand) {
		t.Fatalf("restore missing original command %q:\n%s", wantCommand, got)
	}
	if strings.Contains(got, appliedCommand) {
		t.Fatalf("restore left applied command %q:\n%s", appliedCommand, got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
