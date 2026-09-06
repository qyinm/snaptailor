package cli

import (
	"fmt"
	"html"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/sync"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type checkFlags struct {
	CommonFlags
	ManifestPath string
	CI           bool
	ProjectOnly  bool
}

func newCheckCmd() *cobra.Command {
	var flags checkFlags

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check for drift between team agent manifest and local setup.",
		Long: `Check compares the declarative team manifest (gandalf.toml) with your
local agent environment and reports missing MCP servers, skills, or hooks.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runCheck(cmd, &flags)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	flags.bindFlags(cmd.Flags())
	cmd.Flags().StringVar(&flags.ManifestPath, "manifest", "", "Path to gandalf.toml (default: search project root)")
	cmd.Flags().BoolVar(&flags.CI, "ci", false, "Exit with non-zero status code if drift or errors are detected")
	cmd.Flags().BoolVar(&flags.ProjectOnly, "project-only", false, "Restrict check to repository files without checking user home directory")

	return cmd
}

func runCheck(cmd *cobra.Command, flags *checkFlags) int {
	runtime, snapErr := resolveRuntime(&flags.CommonFlags)
	if snapErr != nil {
		return writeError(cmd.ErrOrStderr(), snapErr)
	}

	manifestPath := flags.ManifestPath
	if manifestPath == "" {
		found, err := manifest.FindManifestFile(runtime.ProjectPath)
		if err != nil {
			return writeError(cmd.ErrOrStderr(), &types.SnapError{
				Code:    "MANIFEST_NOT_FOUND",
				Problem: "No team manifest file found in project",
				Cause:   err.Error(),
				Fix:     "Run 'gandalf init' to create a gandalf.toml in this repository",
			})
		}
		manifestPath = found
	}

	parseOpts := &manifest.ParseOptions{}
	if flags.projectScoped() {
		parseOpts.NoInterpolate = true
	}

	res, err := manifest.LoadManifest(manifestPath, parseOpts)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "MANIFEST_PARSE_ERROR",
			Problem: "Failed to parse manifest file",
			Cause:   err.Error(),
			Fix:     "Check syntax of gandalf.toml",
		})
	}

	validationErrs := manifest.Validate(res.Manifest, runtime.ProjectPath)
	if len(validationErrs) > 0 {
		var msgs []string
		for _, v := range validationErrs {
			msgs = append(msgs, v.Error())
		}
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "MANIFEST_VALIDATION_ERROR",
			Problem: "Manifest validation failed",
			Cause:   strings.Join(msgs, "; "),
			Fix:     validationErrs[0].Fix,
		})
	}

	var drift *sync.DriftReport
	if flags.projectScoped() {
		var err error
		drift, err = sync.DetectProjectDrift(res.Manifest, runtime.ProjectPath)
		if err != nil {
			return writeError(cmd.ErrOrStderr(), &types.SnapError{
				Code:    "PROJECT_DRIFT_ERROR",
				Problem: "Failed to perform project drift check",
				Cause:   err.Error(),
			})
		}
	} else {
		scanOptions := &types.ScanOptions{
			ProjectPath: runtime.ProjectPath,
			HomeDir:     runtime.HomeDir,
			StoreDir:    runtime.StoreDir,
		}
		baseScan := scan.ScanProject(scanOptions)

		var err error
		drift, err = sync.DetectDrift(res.Manifest, runtime.ProjectPath, runtime.HomeDir, baseScan.Evidence)
		if err != nil {
			return writeError(cmd.ErrOrStderr(), &types.SnapError{
				Code:    "DRIFT_CHECK_ERROR",
				Problem: "Failed to perform drift check",
				Cause:   err.Error(),
			})
		}
	}

	// Always attempt to write to $GITHUB_STEP_SUMMARY if present in CI
	writeGitHubStepSummary(res.Manifest, drift)

	if flags.JSON {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"manifest": res.Manifest,
			"drift":    drift,
		})
	}

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "📦 Team Manifest: %s (version: %s)\n", res.Manifest.Name, res.Manifest.Version)
	_, _ = fmt.Fprintf(out, "🎯 Target Agents: %s\n", formatAgentsList(res.Manifest.Agents))
	_, _ = fmt.Fprintln(out)

	if drift.InSync {
		_, _ = fmt.Fprintln(out, "✅ [IN SYNC] All agent configurations match the team manifest!")
		return 0
	}

	_, _ = fmt.Fprintln(out, "⚠️  [DRIFT DETECTED] The following items are missing or out of sync:")
	for i, item := range drift.Items {
		agentPrefix := ""
		if item.Agent != "" {
			agentPrefix = fmt.Sprintf("[%s] ", item.Agent)
		}
		_, _ = fmt.Fprintf(out, "  [%d] %s%s: %s (%s)\n", i+1, agentPrefix, item.Kind, item.Name, item.TargetFile)
		if item.Details != "" {
			_, _ = fmt.Fprintf(out, "      ↳ %s\n", item.Details)
		}
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, flags.applyHint())

	if flags.CI {
		// Output GitHub workflow command annotations with proper property and data escaping
		for _, item := range drift.Items {
			fileArg := ""
			if item.TargetFile != "" {
				fileArg = fmt.Sprintf("file=%s,", escapeWorkflowProperty(item.TargetFile))
			}
			msg := fmt.Sprintf("%s: %s", item.Name, item.Details)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "::error %stitle=%s::%s\n", fileArg, escapeWorkflowProperty("Agent Environment Drift"), escapeWorkflowData(msg))
		}
		return 1
	}

	return 0
}

func (f *checkFlags) projectScoped() bool {
	return f.ProjectOnly || (f.CI && os.Getenv("GITHUB_ACTIONS") == "true")
}

func (f *checkFlags) applyHint() string {
	if f.projectScoped() {
		return "💡 Run 'gandalf apply --project-only' to synchronize repository agent configs."
	}
	return "💡 Run 'gandalf apply' to synchronize your agent environment."
}

func writeGitHubStepSummary(m *manifest.Manifest, drift *sync.DriftReport) {
	summaryFile := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryFile == "" {
		return
	}

	f, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	var sb strings.Builder
	sb.WriteString("### 🛡️ Gandalf Agent Environment Check\n\n")

	var agentStrs []string
	for _, a := range m.Agents {
		agentStrs = append(agentStrs, escapeMarkdownTableCell(string(a)))
	}
	sb.WriteString(fmt.Sprintf("**Manifest:** `%s` (v%s) | **Target Agents:** `%s`\n\n", escapeMarkdownTableCell(m.Name), escapeMarkdownTableCell(m.Version), strings.Join(agentStrs, ", ")))

	if drift.InSync {
		sb.WriteString("✅ **All agent configurations are in sync with the team manifest!**\n\n")
	} else {
		sb.WriteString("⚠️ **Configuration drift or missing requirements detected:**\n\n")
		sb.WriteString("| # | Kind | Target / Name | Details |\n")
		sb.WriteString("| :---: | :--- | :--- | :--- |\n")
		for i, item := range drift.Items {
			kindBadge := fmt.Sprintf("`%s`", escapeMarkdownTableCell(string(item.Kind)))
			target := item.TargetFile
			if target == "" {
				target = item.Name
			}
			nameEsc := escapeMarkdownTableCell(item.Name)
			targetEsc := escapeMarkdownTableCell(target)
			detailsEsc := escapeMarkdownTableCell(item.Details)
			sb.WriteString(fmt.Sprintf("| %d | %s | **%s** (`%s`) | %s |\n", i+1, kindBadge, nameEsc, targetEsc, detailsEsc))
		}
		sb.WriteString("\n> 💡 *Run `gandalf apply --project-only` to synchronize repository agent configs, or `gandalf apply` for user-home.*\n\n")
	}

	_, _ = f.WriteString(sb.String())
}

// escapeWorkflowProperty escapes special characters for GitHub Actions workflow command properties (file, title, etc.).
func escapeWorkflowProperty(s string) string {
	r := strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
		":", "%3A",
		",", "%2C",
	)
	return r.Replace(s)
}

// escapeWorkflowData escapes special characters for GitHub Actions workflow command data/messages.
func escapeWorkflowData(s string) string {
	r := strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	)
	return r.Replace(s)
}

// escapeMarkdownTableCell escapes content intended for Markdown table cells to prevent table breakage and markup injection.
func escapeMarkdownTableCell(s string) string {
	escaped := html.EscapeString(s)
	r := strings.NewReplacer(
		"|", "&#124;",
		"\r\n", "<br>",
		"\n", "<br>",
		"\r", "",
	)
	return r.Replace(escaped)
}
