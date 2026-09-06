package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

// RunImport launches the interactive export wizard: it scans existing agent
// configurations, lets the user toggle which MCP servers and skills to adopt,
// previews the prospective gandalf.toml with masked secrets, and writes on
// confirm. Returns a process exit code (0 on success or user cancel).
func RunImport(runtime types.RuntimeOptions, opts importer.ImportOptions) int {
	if opts.ProjectPath == "" {
		opts.ProjectPath = runtime.ProjectPath
	}
	if opts.HomeDir == "" {
		opts.HomeDir = runtime.HomeDir
	}

	final, err := tea.NewProgram(NewImportApp(runtime, opts), tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gandalf export tui failed: %v\n", err)
		return 1
	}

	app, ok := final.(ImportApp)
	if !ok {
		return 0
	}
	switch {
	case app.Cancelled():
		fmt.Fprintln(os.Stdout, "Export cancelled. No files were written.")
		return 0
	case app.Failed():
		if app.Err() != nil {
			fmt.Fprintf(os.Stderr, "Export failed: %v\n", app.Err())
		}
		return 1
	}
	return 0
}
