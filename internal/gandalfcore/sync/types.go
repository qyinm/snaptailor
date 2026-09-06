package sync

import (
	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// DriftKind classifies the type of difference found.
type DriftKind string

const (
	DriftMissingMCPServer      DriftKind = "missing_mcp_server"
	DriftMissingSkill          DriftKind = "missing_skill"
	DriftMissingHook           DriftKind = "missing_hook"
	DriftOutdatedSkill         DriftKind = "outdated_skill"
	DriftOutdatedConfig        DriftKind = "outdated_config"
	DriftMissingSkillFile      DriftKind = "missing_skill_file"
	DriftMissingEnvTemplate    DriftKind = "missing_env_template"
	DriftUnsyncedProjectConfig DriftKind = "unsynced_project_config"
)

// DriftItem is a single drift entry between manifest and local setup.
type DriftItem struct {
	Agent       types.AgentID `json:"agent"`
	Kind        DriftKind     `json:"kind"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	TargetFile  string        `json:"targetFile"`
	Details     string        `json:"details,omitempty"`
}

// DriftReport summarizes the drift analysis.
type DriftReport struct {
	InSync       bool             `json:"inSync"`
	ProjectName  string           `json:"projectName"`
	TargetAgents []types.AgentID  `json:"targetAgents"`
	Items        []DriftItem      `json:"items"`
	MissingEnvs  []string         `json:"missingEnvs,omitempty"`
}

const (
	projectMergeJSON = "project_mcp_json"
	projectMergeTOML = "project_mcp_toml"
)

// SyncPlanItem represents an individual action in a sync plan.
type SyncPlanItem struct {
	Agent       types.AgentID      `json:"agent"`
	Kind        types.EvidenceKind `json:"kind"`
	Name        string             `json:"name"`
	Action      string             `json:"action"` // "create", "update", "copy"
	TargetFile  string             `json:"targetFile"`
	SourceFile  string             `json:"sourceFile,omitempty"`
	Content     string             `json:"content,omitempty"`
	BaseContent string             `json:"baseContent,omitempty"`
	MergeKind   string             `json:"mergeKind,omitempty"`
	Description string             `json:"description"`
}

// SyncPlan represents the complete plan to apply a manifest.
type SyncPlan struct {
	Manifest    *manifest.Manifest  `json:"manifest"`
	ProjectRoot string              `json:"projectRoot"`
	HomeDir     string              `json:"homeDir"`
	Scope       types.EvidenceScope `json:"scope,omitempty"`
	Items       []SyncPlanItem      `json:"items"`
	Drift       *DriftReport        `json:"drift"`
}

// SyncApplyResult holds the outcome of applying a sync plan.
type SyncApplyResult struct {
	Success        bool           `json:"success"`
	BackupSnapshot string         `json:"backupSnapshot"`
	AppliedItems   []SyncPlanItem `json:"appliedItems"`
	Errors         []string       `json:"errors,omitempty"`
}
