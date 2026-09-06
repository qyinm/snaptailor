package sync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// ProjectSyncPlanner plans Smart Merge writes into repository agent configs
// inspected by DetectProjectDrift / `gandalf check --project-only`.
type ProjectSyncPlanner struct {
	Manifest    *manifest.Manifest
	ProjectRoot string
	HomeDir     string
}

// CreateProjectSyncPlan generates an execution plan for project-scoped MCP files.
// Callers must pass an uninterpolated manifest so CI templates stay `${ENV}` in git.
func CreateProjectSyncPlan(m *manifest.Manifest, projectRoot, homeDir string) (*SyncPlan, error) {
	return (ProjectSyncPlanner{
		Manifest:    m,
		ProjectRoot: projectRoot,
		HomeDir:     homeDir,
	}).CreatePlan()
}

// CreatePlan writes team MCP servers into project agent files for targeted agents.
func (p ProjectSyncPlanner) CreatePlan() (*SyncPlan, error) {
	if p.Manifest == nil {
		return nil, fmt.Errorf("manifest is required")
	}

	plan := &SyncPlan{
		Manifest:    p.Manifest,
		ProjectRoot: p.ProjectRoot,
		HomeDir:     p.HomeDir,
		Scope:       types.ScopeProject,
	}

	for _, agent := range p.Manifest.Agents {
		switch agent {
		case types.AgentClaudeCode:
			if err := p.addJSONConfig(plan, ".mcp.json", agent); err != nil {
				return nil, err
			}
		case types.AgentCursor:
			if err := p.addJSONConfig(plan, filepath.Join(".cursor", "mcp.json"), agent); err != nil {
				return nil, err
			}
		case types.AgentCodex:
			if err := p.addTOMLConfig(plan, filepath.Join(".codex", "config.toml"), agent); err != nil {
				return nil, err
			}
		case types.AgentOpencode, types.AgentPiAgent, types.AgentProject, types.AgentUnknown:
			continue
		default:
			return nil, fmt.Errorf("unsupported agent in project sync plan: %s", agent)
		}
	}

	return plan, nil
}

// RefreshStaleItems re-merges plan content when a target changed after review.
func (p ProjectSyncPlanner) RefreshStaleItems(plan *SyncPlan) error {
	if plan == nil {
		return fmt.Errorf("plan is required")
	}
	for i := range plan.Items {
		item := &plan.Items[i]
		if item.Action != "update" || item.MergeKind == "" {
			continue
		}
		current, err := readOptionalText(item.TargetFile)
		if err != nil {
			return fmt.Errorf("revalidate %s: %w", item.TargetFile, err)
		}
		if current == item.BaseContent {
			continue
		}
		merged, err := mergeProjectTarget(item.MergeKind, current, plan.Manifest)
		if err != nil {
			return fmt.Errorf("re-merge %s: %w", item.TargetFile, err)
		}
		item.Content = merged
		item.BaseContent = current
	}
	return nil
}

func (p ProjectSyncPlanner) addJSONConfig(plan *SyncPlan, relFile string, agent types.AgentID) error {
	target := filepath.Join(p.ProjectRoot, relFile)
	existing, err := readOptionalText(target)
	if err != nil {
		return fmt.Errorf("read %s: %w", relFile, err)
	}
	if len(p.Manifest.MCPServers) == 0 && existing == "" {
		return nil
	}
	merged, err := ReconcileCursorMCPJSON(existing, p.Manifest)
	if err != nil {
		return fmt.Errorf("merge %s: %w", relFile, err)
	}
	plan.Items = append(plan.Items, SyncPlanItem{
		Agent:       agent,
		Kind:        types.KindAgentConfig,
		Name:        filepath.Base(relFile),
		Action:      "update",
		TargetFile:  target,
		Content:     merged,
		BaseContent: existing,
		MergeKind:   projectMergeJSON,
		Description: fmt.Sprintf("Reconcile MCP servers in project %s", relFile),
	})
	return nil
}

func (p ProjectSyncPlanner) addTOMLConfig(plan *SyncPlan, relFile string, agent types.AgentID) error {
	target := filepath.Join(p.ProjectRoot, relFile)
	existing, err := readOptionalText(target)
	if err != nil {
		return fmt.Errorf("read %s: %w", relFile, err)
	}
	if len(p.Manifest.MCPServers) == 0 && existing == "" {
		return nil
	}
	merged, err := ReconcileCodexConfigTOML(existing, p.Manifest)
	if err != nil {
		return fmt.Errorf("merge %s: %w", relFile, err)
	}
	plan.Items = append(plan.Items, SyncPlanItem{
		Agent:       agent,
		Kind:        types.KindAgentConfig,
		Name:        filepath.Base(relFile),
		Action:      "update",
		TargetFile:  target,
		Content:     merged,
		BaseContent: existing,
		MergeKind:   projectMergeTOML,
		Description: fmt.Sprintf("Reconcile MCP servers in project %s", relFile),
	})
	return nil
}

func mergeProjectTarget(kind, existing string, m *manifest.Manifest) (string, error) {
	switch kind {
	case projectMergeJSON:
		return ReconcileCursorMCPJSON(existing, m)
	case projectMergeTOML:
		return ReconcileCodexConfigTOML(existing, m)
	default:
		return "", fmt.Errorf("unknown project merge kind %q", kind)
	}
}

func readOptionalText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}
