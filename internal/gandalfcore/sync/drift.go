package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// DetectDrift compares the team manifest against the discovered local environment items.
func DetectDrift(m *manifest.Manifest, projectRoot, homeDir string, discovered []types.DiscoveredItem) (*DriftReport, error) {
	report := &DriftReport{
		InSync:       true,
		ProjectName:  m.Name,
		TargetAgents: m.Agents,
		Items:        nil,
	}

	// Map existing discovered items by Agent -> Kind -> Name
	existingMCP := make(map[types.AgentID]map[string]bool)
	existingSkills := make(map[types.AgentID]map[string]bool)

	for _, a := range m.Agents {
		existingMCP[a] = make(map[string]bool)
		existingSkills[a] = make(map[string]bool)
	}

	for _, item := range discovered {
		if item.Name == nil {
			continue
		}
		name := *item.Name
		if item.Kind == types.KindMcpServer {
			if m, ok := existingMCP[item.Agent]; ok {
				m[name] = true
			}
		} else if item.Kind == types.KindSkill {
			if m, ok := existingSkills[item.Agent]; ok {
				m[name] = true
			}
		}
	}

	// Check MCP servers for each target agent
	for srvName, srv := range m.MCPServers {
		for _, agent := range m.Agents {
			if !existingMCP[agent][srvName] {
				report.InSync = false
				targetFile := ""
				switch agent {
				case types.AgentClaudeCode:
					targetFile = filepath.Join(homeDir, ".claude", "settings.json")
				case types.AgentCodex:
					targetFile = filepath.Join(homeDir, ".codex", "config.toml")
				case types.AgentCursor:
					targetFile = filepath.Join(homeDir, ".cursor", "mcp.json")
				}

				report.Items = append(report.Items, DriftItem{
					Agent:       agent,
					Kind:        DriftMissingMCPServer,
					Name:        srvName,
					Description: srv.Description,
					TargetFile:  targetFile,
					Details:     "MCP server is not configured in local agent setup",
				})
			}
		}
	}

	// Check Skills for each target agent
	for _, skill := range m.Skills {
		for _, agent := range m.Agents {
			var destSkillDir string
			switch agent {
			case types.AgentClaudeCode:
				destSkillDir = filepath.Join(homeDir, ".claude", "skills", skill.Name)
			case types.AgentCodex:
				destSkillDir = filepath.Join(homeDir, ".codex", "skills", skill.Name)
			case types.AgentCursor:
				destSkillDir = filepath.Join(homeDir, ".cursor", "skills", skill.Name)
			}

			if _, err := os.Stat(destSkillDir); os.IsNotExist(err) {
				report.InSync = false
				report.Items = append(report.Items, DriftItem{
					Agent:       agent,
					Kind:        DriftMissingSkill,
					Name:        skill.Name,
					Description: skill.Description,
					TargetFile:  destSkillDir,
					Details:     "Skill directory is missing in local agent home",
				})
			}
		}
	}

	return report, nil
}

var envVarRegex = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)(?::-([^}]*))?\}`)

func extractEnvsFromString(s string) []string {
	var vars []string
	matches := envVarRegex.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		if len(m) >= 2 {
			vars = append(vars, m[1])
		}
	}
	return vars
}

// DetectProjectDrift checks for drift strictly within project repository files,
// suitable for CI runners where user home directory configs do not exist.
func DetectProjectDrift(m *manifest.Manifest, projectRoot string) (*DriftReport, error) {
	report := &DriftReport{
		InSync:       true,
		ProjectName:  m.Name,
		TargetAgents: m.Agents,
		Items:        nil,
	}

	// 1. Verify that all referenced ${VAR} in MCPServers are declared in EnvTemplate
	seenMissingEnvs := make(map[string]bool)
	for srvName, srv := range m.MCPServers {
		var referencedVars []string
		referencedVars = append(referencedVars, extractEnvsFromString(srv.Command)...)
		for _, arg := range srv.Args {
			referencedVars = append(referencedVars, extractEnvsFromString(arg)...)
		}
		for _, v := range srv.Env {
			referencedVars = append(referencedVars, extractEnvsFromString(v)...)
		}
		for _, v := range srv.Headers {
			referencedVars = append(referencedVars, extractEnvsFromString(v)...)
		}
		if authStr, ok := srv.Auth.(string); ok {
			referencedVars = append(referencedVars, extractEnvsFromString(authStr)...)
		}

		for _, v := range referencedVars {
			if _, exists := m.EnvTemplate[v]; !exists && !seenMissingEnvs[v] {
				seenMissingEnvs[v] = true
				report.InSync = false
				report.MissingEnvs = append(report.MissingEnvs, v)
				report.Items = append(report.Items, DriftItem{
					Kind:        DriftMissingEnvTemplate,
					Name:        v,
					TargetFile:  "gandalf.toml",
					Description: fmt.Sprintf("Missing [env_template.%s] definition", v),
					Details:     fmt.Sprintf("Environment variable '${%s}' is referenced in MCP server '%s' but missing from [env_template]", v, srvName),
				})
			}
		}
	}

	cleanProjectRoot, err := filepath.EvalSymlinks(filepath.Clean(projectRoot))
	if err != nil {
		cleanProjectRoot = filepath.Clean(projectRoot)
	}

	// 2. Verify that all declared skills exist in project and contain SKILL.md without escaping projectRoot
	for _, skill := range m.Skills {
		relPath := skill.Source
		if relPath == "" {
			relPath = filepath.Join(".gandalf", "skills", skill.Name)
		}

		if pathconfinement.PathHasTraversal(relPath) {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftMissingSkill,
				Name:        skill.Name,
				Description: skill.Description,
				TargetFile:  relPath,
				Details:     fmt.Sprintf("Skill path '%s' contains traversal elements", relPath),
			})
			continue
		}

		skillDir := filepath.Join(projectRoot, filepath.Clean(relPath))
		if !pathconfinement.IsStrictlyUnder(filepath.Clean(skillDir), filepath.Clean(projectRoot)) {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftMissingSkill,
				Name:        skill.Name,
				Description: skill.Description,
				TargetFile:  relPath,
				Details:     fmt.Sprintf("Skill path '%s' escapes project root", relPath),
			})
			continue
		}

		// Security: resolve symlinks and ensure real path is strictly under projectRoot
		realSkillDir, err := filepath.EvalSymlinks(skillDir)
		if err != nil {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftMissingSkill,
				Name:        skill.Name,
				Description: skill.Description,
				TargetFile:  relPath,
				Details:     fmt.Sprintf("Skill directory '%s' does not exist or has invalid link: %v", relPath, err),
			})
			continue
		}

		fi, err := os.Stat(realSkillDir)
		if err != nil || !fi.IsDir() {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftMissingSkill,
				Name:        skill.Name,
				Description: skill.Description,
				TargetFile:  relPath,
				Details:     fmt.Sprintf("Skill path '%s' is not a valid directory", relPath),
			})
			continue
		}

		if !pathconfinement.IsStrictlyUnder(realSkillDir, cleanProjectRoot) && realSkillDir != cleanProjectRoot {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftMissingSkill,
				Name:        skill.Name,
				Description: skill.Description,
				TargetFile:  relPath,
				Details:     fmt.Sprintf("Skill directory '%s' links outside repository boundary", relPath),
			})
			continue
		}

		skillMD := filepath.Join(skillDir, "SKILL.md")
		realSkillMD, err := filepath.EvalSymlinks(skillMD)
		if err != nil {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftMissingSkillFile,
				Name:        skill.Name,
				Description: skill.Description,
				TargetFile:  filepath.Join(relPath, "SKILL.md"),
				Details:     fmt.Sprintf("SKILL.md is missing inside skill directory '%s'", relPath),
			})
			continue
		}

		if !pathconfinement.IsStrictlyUnder(realSkillMD, cleanProjectRoot) {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftMissingSkillFile,
				Name:        skill.Name,
				Description: skill.Description,
				TargetFile:  filepath.Join(relPath, "SKILL.md"),
				Details:     fmt.Sprintf("SKILL.md in '%s' links outside repository boundary", relPath),
			})
		}
	}

	targetsAgent := func(agent types.AgentID) bool {
		for _, a := range m.Agents {
			if a == agent {
				return true
			}
		}
		return false
	}

	// 3. Check project-level configs only for agents targeted by the manifest
	// Claude Code -> .mcp.json
	if targetsAgent(types.AgentClaudeCode) {
		checkJSONProjectConfig(report, projectRoot, ".mcp.json", m.MCPServers)
	}

	// Cursor -> .cursor/mcp.json
	if targetsAgent(types.AgentCursor) {
		checkJSONProjectConfig(report, projectRoot, filepath.Join(".cursor", "mcp.json"), m.MCPServers)
	}

	// Codex -> .codex/config.toml
	if targetsAgent(types.AgentCodex) {
		relCodex := filepath.Join(".codex", "config.toml")
		codexPath := filepath.Join(projectRoot, relCodex)
		data, err := os.ReadFile(codexPath)
		if err != nil {
			if !os.IsNotExist(err) {
				report.InSync = false
				report.Items = append(report.Items, DriftItem{
					Kind:        DriftOutdatedConfig,
					TargetFile:  relCodex,
					Description: fmt.Sprintf("Failed to read project config '%s'", relCodex),
					Details:     err.Error(),
				})
			} else if len(m.MCPServers) > 0 {
				// Target agent file is completely missing while manifest declares servers
				compareProjectServers(report, relCodex, m.MCPServers, make(map[string]manifest.MCPServerDef))
			}
		} else {
			parsedServers, err := importer.ParseCodexConfigTOML(data)
			if err != nil {
				report.InSync = false
				report.Items = append(report.Items, DriftItem{
					Kind:        DriftOutdatedConfig,
					TargetFile:  relCodex,
					Description: fmt.Sprintf("Malformed project config '%s'", relCodex),
					Details:     fmt.Sprintf("Failed to parse Codex config '%s': %v", relCodex, err),
				})
			} else {
				compareProjectServers(report, relCodex, m.MCPServers, parsedServers)
			}
		}
	}

	return report, nil
}

func checkJSONProjectConfig(report *DriftReport, projectRoot, relFile string, expectedServers map[string]manifest.MCPServerDef) {
	filePath := filepath.Join(projectRoot, relFile)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftOutdatedConfig,
				TargetFile:  relFile,
				Description: fmt.Sprintf("Failed to read project config '%s'", relFile),
				Details:     err.Error(),
			})
		} else if len(expectedServers) > 0 {
			// Target agent file is completely missing while manifest declares servers
			compareProjectServers(report, relFile, expectedServers, make(map[string]manifest.MCPServerDef))
		}
		return
	}

	parsedServers, err := importer.ParseStandardJSONMCPServers(data)
	if err != nil {
		report.InSync = false
		report.Items = append(report.Items, DriftItem{
			Kind:        DriftOutdatedConfig,
			TargetFile:  relFile,
			Description: fmt.Sprintf("Malformed project config '%s'", relFile),
			Details:     fmt.Sprintf("Failed to parse mcpServers in '%s': %v", relFile, err),
		})
		return
	}

	compareProjectServers(report, relFile, expectedServers, parsedServers)
}

func compareProjectServers(report *DriftReport, targetFile string, expectedServers map[string]manifest.MCPServerDef, actualServers map[string]manifest.MCPServerDef) {
	for srvName, exp := range expectedServers {
		act, ok := actualServers[srvName]
		if !ok {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftUnsyncedProjectConfig,
				Name:        srvName,
				TargetFile:  targetFile,
				Description: fmt.Sprintf("MCP server '%s' declared in gandalf.toml is missing from project '%s'", srvName, targetFile),
				Details:     "Project agent configuration is out of sync with gandalf.toml (run 'gandalf apply --project-only' to sync)",
			})
			continue
		}

		if !isMCPServerEqual(exp, act) {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftOutdatedConfig,
				Name:        srvName,
				TargetFile:  targetFile,
				Description: fmt.Sprintf("MCP server '%s' in '%s' has modified settings differing from gandalf.toml", srvName, targetFile),
				Details:     "Server configuration differs from gandalf.toml (run 'gandalf apply --project-only' to sync)",
			})
		}
	}

	for srvName := range actualServers {
		if _, ok := expectedServers[srvName]; !ok {
			report.InSync = false
			report.Items = append(report.Items, DriftItem{
				Kind:        DriftUnsyncedProjectConfig,
				Name:        srvName,
				TargetFile:  targetFile,
				Description: fmt.Sprintf("Unmanaged MCP server '%s' in project '%s' is not declared in gandalf.toml", srvName, targetFile),
				Details:     "Project file contains extra MCP servers not tracked by team manifest (run 'gandalf apply --project-only' to drop them, or 'gandalf export' to add them)",
			})
		}
	}
}

func isMCPServerEqual(expected, actual manifest.MCPServerDef) bool {
	if expected.Type != actual.Type {
		return false
	}
	if expected.Command != actual.Command {
		return false
	}
	if !slicesEqual(expected.Args, actual.Args) {
		return false
	}
	if expected.URL != actual.URL {
		return false
	}
	if expected.EnvFile != actual.EnvFile {
		return false
	}
	if expected.Disabled != actual.Disabled {
		return false
	}
	if !mapsEqual(expected.Env, actual.Env) {
		return false
	}
	if !mapsEqual(expected.Headers, actual.Headers) {
		return false
	}
	if !isAuthEqual(expected.Auth, actual.Auth) {
		return false
	}
	return true
}

func isAuthEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if reflect.DeepEqual(a, b) {
		return true
	}
	// Normalize via JSON marshaling to handle differences in int vs float64 or map key ordering
	aBytes, errA := json.Marshal(a)
	bBytes, errB := json.Marshal(b)
	if errA == nil && errB == nil {
		var aNorm, bNorm any
		if json.Unmarshal(aBytes, &aNorm) == nil && json.Unmarshal(bBytes, &bNorm) == nil {
			return reflect.DeepEqual(aNorm, bNorm)
		}
	}
	return false
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
