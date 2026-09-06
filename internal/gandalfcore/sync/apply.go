package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/fsutil"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/snapshot"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ApplySyncPlan applies the planned changes to local agent configurations and skill directories.
func ApplySyncPlan(plan *SyncPlan, roots *pathconfinement.Roots, storeDir string) (*SyncApplyResult, error) {
	if len(plan.Items) == 0 {
		return &SyncApplyResult{
			Success:      true,
			AppliedItems: nil,
		}, nil
	}

	// 1. Verify Path Confinement for all targets
	for _, item := range plan.Items {
		if roots != nil {
			if _, err := pathconfinement.ValidateConstrainedWritePath(item.TargetFile, roots); err != nil {
				return nil, fmt.Errorf("confinement check failed for '%s': %w", item.TargetFile, err)
			}
		}
	}

	if plan.Scope == types.ScopeProject {
		if err := (ProjectSyncPlanner{Manifest: plan.Manifest, ProjectRoot: plan.ProjectRoot, HomeDir: plan.HomeDir}).RefreshStaleItems(plan); err != nil {
			return nil, err
		}
	}

	// 2. Create Pre-apply backup snapshot
	backupName := fmt.Sprintf("preapply-manifest-%s", time.Now().Format("20060102-150405"))
	var backupAgent *types.AgentID
	if plan.Manifest != nil && len(plan.Manifest.Agents) == 1 {
		backupAgent = &plan.Manifest.Agents[0]
	}
	backupScope := types.ScopeUser
	if plan.Scope != "" {
		backupScope = plan.Scope
	}
	if storeDir != "" {
		state, err := snapshot.CaptureCurrentState(&types.RuntimeOptions{
			ProjectPath:    plan.ProjectRoot,
			HomeDir:        plan.HomeDir,
			StoreDir:       storeDir,
			Agent:          backupAgent,
			Scope:          &backupScope,
			CaptureContent: true,
		}, backupName)
		if err == nil && state != nil && projectSnapshotCoversTargets(state.Snapshot, plan) {
			if writeErr := store.WriteSnapshot(storeDir, store.StoreSnapshotFrom(state.Snapshot), backupAgent); writeErr != nil {
				backupName = ""
			}
		} else {
			backupName = ""
		}
	} else {
		backupName = ""
	}

	var applied []SyncPlanItem
	var errors []string

	// 3. Apply items
	for _, item := range plan.Items {
		switch item.Action {
		case "update":
			// Ensure parent directory exists
			dir := filepath.Dir(item.TargetFile)
			if err := os.MkdirAll(dir, 0755); err != nil {
				errors = append(errors, fmt.Sprintf("create dir '%s': %v", dir, err))
				continue
			}

			if err := fsutil.WriteTextAtomically(item.TargetFile, item.Content, 0644); err != nil {
				errors = append(errors, fmt.Sprintf("write file '%s': %v", item.TargetFile, err))
				continue
			}
			applied = append(applied, item)

		case "copy":
			if item.SourceFile == "" {
				continue
			}
			if err := copyDirOrFile(item.SourceFile, item.TargetFile); err != nil {
				errors = append(errors, fmt.Sprintf("copy '%s' to '%s': %v", item.SourceFile, item.TargetFile, err))
				continue
			}
			applied = append(applied, item)
		}
	}

	success := len(errors) == 0
	return &SyncApplyResult{
		Success:        success,
		BackupSnapshot: backupName,
		AppliedItems:   applied,
		Errors:         errors,
	}, nil
}

func projectSnapshotCoversTargets(snap types.Snapshot, plan *SyncPlan) bool {
	if plan == nil || plan.Scope != types.ScopeProject {
		return true
	}
	captured := make(map[string]bool)
	for _, entry := range snap.Content {
		if entry.CaptureStatus != "captured" || entry.Content == nil {
			continue
		}
		captured[filepath.ToSlash(entry.SourcePath)] = true
		captured[filepath.ToSlash(entry.RestorePath)] = true
	}
	for _, item := range plan.Items {
		if item.Action != "update" {
			continue
		}
		if item.BaseContent == "" {
			continue
		}
		rel := item.TargetFile
		if plan.ProjectRoot != "" {
			if next, err := filepath.Rel(plan.ProjectRoot, item.TargetFile); err == nil {
				rel = next
			}
		}
		if !captured[filepath.ToSlash(rel)] {
			return false
		}
	}
	return true
}

func copyDirOrFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	sourceInfo, err := os.Stat(src)
	if err == nil {
		_ = os.Chmod(dst, sourceInfo.Mode())
	}
	return nil
}

func copyDir(src, dst string) error {
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, sourceInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}
