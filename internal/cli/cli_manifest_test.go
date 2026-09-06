package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIInitCheckApply(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	projectDir := filepath.Join(tempDir, "project")
	storeDir := filepath.Join(tempDir, "store")

	_ = os.MkdirAll(homeDir, 0755)
	_ = os.MkdirAll(projectDir, 0755)
	_ = os.MkdirAll(storeDir, 0755)

	// 1. Test gandalf init
	initCmd := newInitCmd()
	var initBuf bytes.Buffer
	initCmd.SetOut(&initBuf)
	initCmd.SetErr(&initBuf)
	initCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--name", "test-project"})

	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v, output: %s", err, initBuf.String())
	}

	manifestFile := filepath.Join(projectDir, "gandalf.toml")
	if _, err := os.Stat(manifestFile); err != nil {
		t.Fatalf("gandalf.toml was not generated: %v", err)
	}

	// 2. Test gandalf check (should detect drift before apply)
	checkCmd := newCheckCmd()
	var checkBuf bytes.Buffer
	checkCmd.SetOut(&checkBuf)
	checkCmd.SetErr(&checkBuf)
	checkCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir})

	if err := checkCmd.Execute(); err != nil {
		t.Fatalf("check failed: %v", err)
	}

	if !strings.Contains(checkBuf.String(), "DRIFT DETECTED") {
		t.Errorf("expected drift detected output, got: %s", checkBuf.String())
	}

	// 3. Test gandalf apply --dry-run
	dryApplyCmd := newApplyCmd()
	var dryBuf bytes.Buffer
	dryApplyCmd.SetOut(&dryBuf)
	dryApplyCmd.SetErr(&dryBuf)
	dryApplyCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--dry-run"})

	if err := dryApplyCmd.Execute(); err != nil {
		t.Fatalf("dry-run apply failed: %v", err)
	}
	if !strings.Contains(dryBuf.String(), "Dry-run mode") {
		t.Errorf("expected dry-run message, got: %s", dryBuf.String())
	}

	// 4. Test gandalf apply --yes
	applyCmd := newApplyCmd()
	var applyBuf bytes.Buffer
	applyCmd.SetOut(&applyBuf)
	applyCmd.SetErr(&applyBuf)
	applyCmd.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--yes"})

	if err := applyCmd.Execute(); err != nil {
		t.Fatalf("apply failed: %v, output: %s", err, applyBuf.String())
	}

	if !strings.Contains(applyBuf.String(), "Successfully synchronized") {
		t.Errorf("expected success message, got: %s", applyBuf.String())
	}

	// 5. Check again (should be in sync now)
	checkCmd2 := newCheckCmd()
	var checkBuf2 bytes.Buffer
	checkCmd2.SetOut(&checkBuf2)
	checkCmd2.SetErr(&checkBuf2)
	checkCmd2.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir})

	if err := checkCmd2.Execute(); err != nil {
		t.Fatalf("second check failed: %v", err)
	}

	if !strings.Contains(checkBuf2.String(), "IN SYNC") {
		t.Errorf("expected in sync output, got: %s", checkBuf2.String())
	}
}

func TestCLIApplyProjectOnly_AlignsCheckProjectOnly(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	projectDir := filepath.Join(tempDir, "project")
	storeDir := filepath.Join(tempDir, "store")
	_ = os.MkdirAll(homeDir, 0755)
	_ = os.MkdirAll(projectDir, 0755)
	_ = os.MkdirAll(storeDir, 0755)

	t.Setenv("APP_ENV", "should-not-leak-into-project-files")

	manifestContent := `
version = "1.0"
name = "ci-align"
agents = ["claude-code", "codex"]

[mcp_servers.echo]
command = "echo"
args = ["${APP_ENV}"]

[env_template]
APP_ENV = "production"
`
	if err := os.WriteFile(filepath.Join(projectDir, "gandalf.toml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	homeApply := newApplyCmd()
	var homeBuf bytes.Buffer
	homeApply.SetOut(&homeBuf)
	homeApply.SetErr(&homeBuf)
	homeApply.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--yes"})
	if err := homeApply.Execute(); err != nil {
		t.Fatalf("home apply failed: %v, output: %s", err, homeBuf.String())
	}

	projectCheck := newCheckCmd()
	var driftBuf, driftErr bytes.Buffer
	projectCheck.SetOut(&driftBuf)
	projectCheck.SetErr(&driftErr)
	projectCheck.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--project-only", "--ci"})
	if err := projectCheck.Execute(); err == nil {
		t.Fatalf("expected project-only check to fail after home-only apply, stdout: %s", driftBuf.String())
	}
	if !strings.Contains(driftBuf.String(), "DRIFT DETECTED") {
		t.Errorf("expected drift after home-only apply, got: %s", driftBuf.String())
	}
	if !strings.Contains(driftBuf.String(), "gandalf apply --project-only") {
		t.Errorf("expected project-only apply hint, got: %s", driftBuf.String())
	}

	projectApply := newApplyCmd()
	var applyBuf bytes.Buffer
	projectApply.SetOut(&applyBuf)
	projectApply.SetErr(&applyBuf)
	projectApply.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--project-only", "--yes"})
	if err := projectApply.Execute(); err != nil {
		t.Fatalf("project-only apply failed: %v, output: %s", err, applyBuf.String())
	}

	mcpBytes, err := os.ReadFile(filepath.Join(projectDir, ".mcp.json"))
	if err != nil {
		t.Fatalf("expected project .mcp.json after apply --project-only: %v", err)
	}
	mcp := string(mcpBytes)
	if strings.Contains(mcp, "should-not-leak-into-project-files") {
		t.Errorf("project apply must not interpolate process env into git files, got: %s", mcp)
	}
	if !strings.Contains(mcp, "${APP_ENV}") {
		t.Errorf("expected ${APP_ENV} in project .mcp.json, got: %s", mcp)
	}

	if _, err := os.Stat(filepath.Join(projectDir, ".codex", "config.toml")); err != nil {
		t.Fatalf("expected project Codex config after apply --project-only: %v", err)
	}

	checkAfter := newCheckCmd()
	var syncBuf, syncErr bytes.Buffer
	checkAfter.SetOut(&syncBuf)
	checkAfter.SetErr(&syncErr)
	checkAfter.SetArgs([]string{"--project", projectDir, "--home", homeDir, "--store", storeDir, "--project-only", "--ci"})
	if err := checkAfter.Execute(); err != nil {
		t.Fatalf("expected project-only check to pass after apply --project-only: %v\nstdout: %s\nstderr: %s", err, syncBuf.String(), syncErr.String())
	}
	if !strings.Contains(syncBuf.String(), "IN SYNC") {
		t.Errorf("expected IN SYNC, got: %s", syncBuf.String())
	}
}
