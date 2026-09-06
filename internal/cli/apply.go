package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/sync"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

type applyFlags struct {
	CommonFlags
	ManifestPath string
	DryRun       bool
	Yes          bool
	ProjectOnly  bool
}

func newApplyCmd() *cobra.Command {
	var flags applyFlags

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply team agent manifest (gandalf.toml) to local agent environments.",
		Long: `Apply synchronizes the declarative team agent configuration (gandalf.toml)
to your local Codex, Claude Code, and Cursor agent setups with safety pre-apply backup and review.

Default apply writes user-home configs only. Use --project-only to Smart Merge
repository agent files checked by 'gandalf check --project-only' / the CI Action.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exitCode := runApply(cmd, &flags)
			if exitCode != 0 {
				return errExit(exitCode)
			}
			return nil
		},
	}

	flags.bindFlags(cmd.Flags())
	cmd.Flags().StringVar(&flags.ManifestPath, "manifest", "", "Path to gandalf.toml (default: search project root)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Preview planned changes without writing to disk")
	cmd.Flags().BoolVarP(&flags.Yes, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&flags.ProjectOnly, "project-only", false, "Write repository agent configs (.mcp.json, .cursor/mcp.json, .codex/config.toml) instead of user-home")

	return cmd
}

func runApply(cmd *cobra.Command, flags *applyFlags) int {
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
	if flags.ProjectOnly {
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

	if len(res.MissingEnvs) > 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  Warning: Missing required environment variable(s): %s\n", strings.Join(res.MissingEnvs, ", "))
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "   Make sure to export them before running agent tools that require these variables.")
	}

	var plan *sync.SyncPlan
	if flags.ProjectOnly {
		plan, err = sync.CreateProjectSyncPlan(res.Manifest, runtime.ProjectPath, runtime.HomeDir)
	} else {
		scanOptions := &types.ScanOptions{
			ProjectPath: runtime.ProjectPath,
			HomeDir:     runtime.HomeDir,
			StoreDir:    runtime.StoreDir,
		}
		baseScan := scan.ScanProject(scanOptions)
		plan, err = sync.CreateSyncPlan(res.Manifest, runtime.ProjectPath, runtime.HomeDir, baseScan.Evidence)
	}
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "SYNC_PLAN_ERROR",
			Problem: "Failed to generate sync plan",
			Cause:   err.Error(),
		})
	}

	if flags.JSON {
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"manifest": res.Manifest,
			"plan":     plan,
			"dryRun":   flags.DryRun,
		})
	}

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "📦 Team Manifest: %s (version: %s)\n", res.Manifest.Name, res.Manifest.Version)
	_, _ = fmt.Fprintf(out, "🎯 Target Agents: %s\n", formatAgentsList(res.Manifest.Agents))
	_, _ = fmt.Fprintln(out)

	if len(plan.Items) == 0 {
		_, _ = fmt.Fprintln(out, "✨ Local agent setup is already in sync with team manifest. No changes needed.")
		return 0
	}

	_, _ = fmt.Fprintln(out, "🔍 Review Changes before Apply:")
	for i, item := range plan.Items {
		_, _ = fmt.Fprintf(out, "  [%d] [%s] %s (%s)\n", i+1, item.Agent, item.Description, item.TargetFile)
	}
	_, _ = fmt.Fprintln(out)

	if flags.DryRun {
		_, _ = fmt.Fprintln(out, "💡 [Dry-run mode] No changes were written to disk.")
		return 0
	}

	if !flags.Yes {
		_, _ = fmt.Fprint(out, "Apply these changes to local agent configurations? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if strings.ToLower(input) != "y" && strings.ToLower(input) != "yes" {
			_, _ = fmt.Fprintln(out, "Apply cancelled by user.")
			return 0
		}
	}

	roots := pathconfinement.RootsFromPaths(&runtime.HomeDir, &runtime.ProjectPath)
	result, err := sync.ApplySyncPlan(plan, roots, runtime.StoreDir)
	if err != nil {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "SYNC_APPLY_ERROR",
			Problem: "Failed to apply sync plan",
			Cause:   err.Error(),
		})
	}

	if !result.Success {
		return writeError(cmd.ErrOrStderr(), &types.SnapError{
			Code:    "SYNC_PARTIAL_ERROR",
			Problem: "Some sync items failed to apply",
			Cause:   strings.Join(result.Errors, "; "),
		})
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "✅ Successfully synchronized team agent environment!")
	if result.BackupSnapshot != "" {
		_, _ = fmt.Fprintf(out, "🛡️  Pre-apply safety snapshot created: %s\n", result.BackupSnapshot)
		_, _ = fmt.Fprintf(out, "   (To roll back: 'gandalf restore --snapshot %s --apply')\n", result.BackupSnapshot)
	}

	return 0
}

func formatAgentsList(agents []types.AgentID) string {
	var names []string
	for _, a := range agents {
		names = append(names, string(a))
	}
	return strings.Join(names, ", ")
}
