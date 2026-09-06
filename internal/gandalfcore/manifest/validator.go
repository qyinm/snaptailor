package manifest

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/qyinm/gandalf/internal/gandalfcore/agents"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
)

// Validate checks the manifest for schema compliance, semantic validity, and path security.
func Validate(m *Manifest, projectRoot string) []ValidationError {
	var errors []ValidationError

	if m.Version == "" {
		errors = append(errors, ValidationError{
			Field:   "version",
			Problem: "Manifest version is missing",
			Fix:     "Add 'version = \"1.0\"' to the top of gandalf.toml",
		})
	}

	if m.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Problem: "Project/team name is missing",
			Fix:     "Add 'name = \"your-team-project\"' to gandalf.toml",
		})
	}

	if len(m.Agents) == 0 {
		errors = append(errors, ValidationError{
			Field:   "agents",
			Problem: "No target agents specified",
			Fix:     fmt.Sprintf("Specify agents from the supported set: %s", strings.Join(agents.CurrentSupportedNames(), ", ")),
		})
	} else {
		for _, agent := range m.Agents {
			if !agents.IsCurrentSupported(agent) {
				errors = append(errors, ValidationError{
					Field:   "agents",
					Problem: fmt.Sprintf("Agent '%s' is not in the currently supported agent set (supported: %s)", agent, strings.Join(agents.CurrentSupportedNames(), ", ")),
					Fix:     fmt.Sprintf("Use supported agent IDs: %s", strings.Join(agents.CurrentSupportedNames(), ", ")),
				})
			}
		}
	}

	for name, srv := range m.MCPServers {
		if strings.TrimSpace(name) == "" {
			errors = append(errors, ValidationError{
				Field:   "mcp_servers",
				Problem: "MCP server name cannot be empty",
				Fix:     "Provide a valid identifier for the MCP server",
			})
		}
		if srv.Command == "" && srv.URL == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("mcp_servers.%s", name),
				Problem: fmt.Sprintf("MCP server '%s' must specify either 'command' or 'url'", name),
				Fix:     "Add a command (e.g. 'npx') or URL (e.g. 'https://...') to the server definition",
			})
		}
	}

	for i, skill := range m.Skills {
		if skill.Name == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("skills[%d].name", i),
				Problem: "Skill name is missing",
				Fix:     "Provide a name for the skill",
			})
		}

		if skill.Source != "" && projectRoot != "" {
			cleanSource := filepath.Clean(skill.Source)
			fullPath := filepath.Join(projectRoot, cleanSource)

			// Security check: Must not escape project root
			if pathconfinement.PathHasTraversal(skill.Source) || !pathconfinement.IsStrictlyUnder(fullPath, filepath.Clean(projectRoot)) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("skills[%d].source", i),
					Problem: fmt.Sprintf("Skill '%s' source path '%s' escapes project root", skill.Name, skill.Source),
					Fix:     "Place skill inside the project root (e.g. './.gandalf/skills/...')",
				})
			}
		}
	}

	for name, hook := range m.Hooks {
		if hook.Event == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("hooks.%s.event", name),
				Problem: fmt.Sprintf("Hook '%s' is missing 'event'", name),
				Fix:     "Specify an event (e.g. 'before_save', 'on_start')",
			})
		}
		if hook.Command == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("hooks.%s.command", name),
				Problem: fmt.Sprintf("Hook '%s' is missing 'command'", name),
				Fix:     "Specify a command to execute for the hook",
			})
		}
	}

	return errors
}
