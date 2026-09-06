package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/qyinm/gandalf/internal/gandalfcore/importer"
	"github.com/qyinm/gandalf/internal/gandalfcore/manifest"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
	"github.com/qyinm/gandalf/internal/tui/views"
)

// importStep identifies the current wizard screen.
type importStep int

const (
	importStepLoading importStep = iota
	importStepSelect
	importStepPreview
	importStepConfirmOverwrite
	importStepWriting
	importStepDone
	importStepFailed
)

// Messages driving the wizard's async scan and write phases.
type importScanDoneMsg struct {
	result *importer.ImportResult
	err    error
}

type importWriteDoneMsg struct {
	result *importer.ImportResult
	err    error
}

// importGroup is one source-agent section with toggleable items.
type importGroup struct {
	agent string
	scope string
	path  string
	items []importItem
}

type importItem struct {
	kind   string // "server" | "skill"
	name   string
	detail string
}

// ImportApp is the Bubbletea model for the interactive import wizard.
type ImportApp struct {
	runtime types.RuntimeOptions
	opts    importer.ImportOptions

	step   importStep
	width  int
	height int

	result         *importer.ImportResult
	groups         []importGroup
	selServers     map[string]bool
	selSkills      map[string]bool
	cursor         int
	previewOffset  int
	manifestExists bool

	written   *importer.ImportResult
	cancelled bool
	err       error
}

// NewImportApp builds the import wizard model. Detection and reconciliation run
// asynchronously via Init so the UI stays responsive.
func NewImportApp(runtime types.RuntimeOptions, opts importer.ImportOptions) ImportApp {
	if opts.ProjectPath == "" {
		opts.ProjectPath = runtime.ProjectPath
	}
	if opts.HomeDir == "" {
		opts.HomeDir = runtime.HomeDir
	}
	if opts.OutputFile == "" {
		opts.OutputFile = "gandalf.toml"
	}
	return ImportApp{
		runtime:    runtime,
		opts:       opts,
		step:       importStepLoading,
		width:      80,
		height:     24,
		selServers: make(map[string]bool),
		selSkills:  make(map[string]bool),
	}
}

// Cancelled reports whether the user aborted the wizard without writing.
func (a ImportApp) Cancelled() bool { return a.cancelled }

// Failed reports whether the wizard ended in an unrecoverable error.
func (a ImportApp) Failed() bool { return a.step == importStepFailed }

// Err returns the terminal error, if any.
func (a ImportApp) Err() error { return a.err }

// Written returns the import result after a successful write.
func (a ImportApp) Written() *importer.ImportResult { return a.written }

func (a ImportApp) Init() tea.Cmd {
	return scanImportSourcesCmd(a.opts)
}

func scanImportSourcesCmd(opts importer.ImportOptions) tea.Cmd {
	return func() tea.Msg {
		// CandidatesFor honors --from so the wizard previews exactly the
		// custom source RunImport would write.
		candidates, err := importer.CandidatesFor(opts)
		if err != nil {
			return importScanDoneMsg{err: err}
		}
		if len(candidates) == 0 {
			return importScanDoneMsg{err: fmt.Errorf("no agent configurations found to export (checked .cursor/mcp.json, .mcp.json, ~/.cursor/mcp.json, etc.)")}
		}
		result, err := importer.ReconcileSources(opts, candidates)
		return importScanDoneMsg{result: result, err: err}
	}
}

func writeImportCmd(opts importer.ImportOptions, sel *importer.Selection) tea.Cmd {
	return func() tea.Msg {
		writeOpts := opts
		writeOpts.DryRun = false
		writeOpts.Selection = sel
		result, err := importer.RunImport(writeOpts)
		return importWriteDoneMsg{result: result, err: err}
	}
}

func (a ImportApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case importScanDoneMsg:
		if msg.err != nil {
			a.step = importStepFailed
			a.err = msg.err
			return a, nil
		}
		a.result = msg.result
		a.buildGroups()
		if a.itemCount() == 0 {
			a.step = importStepFailed
			a.err = fmt.Errorf("no MCP servers or skills were discovered to export")
			return a, nil
		}
		a.manifestExists = fileExistsIn(a.opts.ProjectPath, a.opts.OutputFile)
		a.step = importStepSelect
		return a, nil

	case importWriteDoneMsg:
		if msg.err != nil {
			a.step = importStepFailed
			a.err = msg.err
			return a, nil
		}
		a.written = msg.result
		a.step = importStepDone
		return a, nil

	case tea.KeyMsg:
		return a.handleKey(msg)
	}
	return a, nil
}

func (a ImportApp) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch a.step {
	case importStepLoading:
		if key == "q" || key == "esc" || key == "ctrl+c" {
			a.cancelled = true
			return a, tea.Quit
		}

	case importStepSelect:
		switch key {
		case "q", "esc", "ctrl+c":
			a.cancelled = true
			return a, tea.Quit
		case "up", "k":
			a.moveCursor(-1)
		case "down", "j":
			a.moveCursor(1)
		case " ":
			a.toggleCurrent()
		case "tab":
			a.previewOffset = 0
			a.step = importStepPreview
		case "enter":
			return a.beginWrite()
		}

	case importStepPreview:
		switch key {
		case "q", "ctrl+c":
			a.cancelled = true
			return a, tea.Quit
		case "tab", "esc":
			a.step = importStepSelect
		case "up", "k":
			a.scrollPreview(-1)
		case "down", "j":
			a.scrollPreview(1)
		case "enter":
			return a.beginWrite()
		}

	case importStepConfirmOverwrite:
		switch key {
		case "y", "Y":
			a.opts.Force = true
			return a.beginWrite()
		case "n", "N", "esc", "q":
			a.step = importStepSelect
		}

	case importStepDone, importStepFailed:
		// Any key exits the terminal screen.
		return a, tea.Quit
	}
	return a, nil
}

// beginWrite transitions into the write phase, asking before overwriting an
// existing manifest unless --force was supplied.
func (a ImportApp) beginWrite() (tea.Model, tea.Cmd) {
	if a.manifestExists && !a.opts.Force {
		a.step = importStepConfirmOverwrite
		return a, nil
	}
	a.step = importStepWriting
	return a, writeImportCmd(a.opts, a.selection())
}

func (a *ImportApp) buildGroups() {
	a.groups = nil
	for _, src := range a.result.Sources {
		group := importGroup{
			agent: agentDisplayName(src.Agent),
			scope: src.Scope,
			path:  src.Path,
		}
		for _, name := range src.ServerNames {
			a.selServers[name] = true
			group.items = append(group.items, importItem{
				kind:   "server",
				name:   name,
				detail: serverDetail(a.result.Manifest.MCPServers[name]),
			})
		}
		for _, name := range src.SkillNames {
			a.selSkills[name] = true
			group.items = append(group.items, importItem{
				kind:   "skill",
				name:   name,
				detail: skillDetail(a.result.Manifest.Skills, name),
			})
		}
		if len(group.items) > 0 {
			a.groups = append(a.groups, group)
		}
	}
}

func agentDisplayName(id types.AgentID) string {
	switch id {
	case types.AgentClaudeCode:
		return "Claude Code"
	case types.AgentCursor:
		return "Cursor"
	case types.AgentCodex:
		return "Codex"
	default:
		s := string(id)
		if s == "" {
			return "Unknown"
		}
		return s
	}
}

func serverDetail(srv manifest.MCPServerDef) string {
	if srv.Command != "" {
		return strings.TrimSpace(srv.Command + " " + strings.Join(srv.Args, " "))
	}
	return srv.URL
}

func skillDetail(skills []manifest.SkillDef, name string) string {
	for _, sk := range skills {
		if sk.Name == name {
			if sk.Description != "" {
				return sk.Description
			}
			return sk.Source
		}
	}
	return ""
}

func (a ImportApp) itemCount() int {
	total := 0
	for _, g := range a.groups {
		total += len(g.items)
	}
	return total
}

// flatItem resolves a flat cursor index to its group/item position.
func (a ImportApp) flatItem(index int) (groupIdx, itemIdx int, ok bool) {
	i := 0
	for gi, g := range a.groups {
		for ii := range g.items {
			if i == index {
				return gi, ii, true
			}
			i++
		}
	}
	return 0, 0, false
}

func (a *ImportApp) moveCursor(delta int) {
	total := a.itemCount()
	if total == 0 {
		return
	}
	a.cursor = ((a.cursor+delta)%total + total) % total
}

func (a *ImportApp) toggleCurrent() {
	gi, ii, ok := a.flatItem(a.cursor)
	if !ok {
		return
	}
	item := a.groups[gi].items[ii]
	if item.kind == "server" {
		a.selServers[item.name] = !a.selServers[item.name]
	} else {
		a.selSkills[item.name] = !a.selSkills[item.name]
	}
}

func (a *ImportApp) scrollPreview(delta int) {
	a.previewOffset += delta
	if a.previewOffset < 0 {
		a.previewOffset = 0
	}
	maxOffset := len(strings.Split(strings.TrimRight(a.previewTOML(), "\n"), "\n")) - 1
	if a.previewOffset > maxOffset {
		a.previewOffset = maxOffset
	}
}

// selection snapshots the current toggle state for the write path.
func (a ImportApp) selection() *importer.Selection {
	servers := make(map[string]bool, len(a.selServers))
	for k, v := range a.selServers {
		servers[k] = v
	}
	skills := make(map[string]bool, len(a.selSkills))
	for k, v := range a.selSkills {
		skills[k] = v
	}
	return &importer.Selection{Servers: servers, Skills: skills}
}

// previewTOML renders the manifest exactly as it would be written with the
// current selection, keeping secrets masked as ${VAR} placeholders.
func (a ImportApp) previewTOML() string {
	if a.result == nil {
		return ""
	}
	return importer.FormatManifestTOML(importer.FilterManifest(a.result.Manifest, a.selection()))
}

func (a ImportApp) maskedCount() int {
	if a.result == nil {
		return 0
	}
	filtered := importer.FilterManifest(a.result.Manifest, a.selection())
	return len(filtered.EnvTemplate)
}

func (a ImportApp) View() string {
	switch a.step {
	case importStepLoading:
		return views.RenderImportLoading("Export agent setup", a.opts.ProjectPath, a.width, a.height)

	case importStepSelect, importStepConfirmOverwrite:
		model := views.ImportSelectModel{
			Title:       "Export agent setup",
			ProjectPath: a.opts.ProjectPath,
			Warnings:    a.result.Warnings,
			Width:       a.width,
			Height:      a.height,
		}
		idx := 0
		for _, g := range a.groups {
			vg := views.ImportGroupView{Agent: g.agent, Scope: g.scope, Path: g.path}
			for _, item := range g.items {
				selected := a.selServers[item.name]
				if item.kind == "skill" {
					selected = a.selSkills[item.name]
				}
				vg.Items = append(vg.Items, views.ImportItemView{
					Kind:     item.kind,
					Name:     item.name,
					Detail:   item.detail,
					Selected: selected,
					Cursor:   idx == a.cursor,
				})
				idx++
			}
			model.Groups = append(model.Groups, vg)
		}
		view := views.RenderImportSelect(model)
		if a.step == importStepConfirmOverwrite {
			prompt := fmt.Sprintf("%s already exists. Overwrite? (y/n)", a.opts.OutputFile)
			return view + "\n" + prompt
		}
		return view

	case importStepPreview:
		return views.RenderImportPreview(views.ImportPreviewModel{
			OutputFile:  a.opts.OutputFile,
			TOML:        a.previewTOML(),
			Offset:      a.previewOffset,
			MaskedCount: a.maskedCount(),
			Width:       a.width,
			Height:      a.height,
		})

	case importStepWriting:
		return views.RenderImportResult(views.ImportResultModel{
			Title:  "Export agent setup",
			Lines:  []string{fmt.Sprintf("Writing %s and mirroring team skills…", a.opts.OutputFile)},
			Hint:   "please wait",
			Width:  a.width,
			Height: a.height,
		})

	case importStepDone:
		res := a.written
		var lines []string
		// Partial-scan failures must not look identical to a complete scan.
		for _, w := range res.Warnings {
			lines = append(lines, "⚠ "+w)
		}
		if len(res.Warnings) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines,
			fmt.Sprintf("Generated %s", a.opts.OutputFile),
			fmt.Sprintf("Discovered %d source(s), %d MCP server(s), %d skill(s)",
				len(res.Sources), len(res.Manifest.MCPServers), len(res.Manifest.Skills)),
		)
		if len(res.ExtractedEnvs) > 0 {
			lines = append(lines, fmt.Sprintf("Secret protection: templated %d sensitive variable(s) into [env_template]", len(res.ExtractedEnvs)))
		}
		if len(res.MirroredSkills) > 0 {
			lines = append(lines, fmt.Sprintf("Mirrored %d skill(s) into .gandalf/skills/", len(res.MirroredSkills)))
		}
		lines = append(lines, "",
			"Next steps:",
			fmt.Sprintf("  1. Review '%s' and configure values in [env_template].", a.opts.OutputFile),
			"  2. Run 'gandalf check' to verify parity locally and in CI.",
			"  3. Team members can run 'gandalf apply' to sync their local agent setup.",
		)
		return views.RenderImportResult(views.ImportResultModel{
			Title:  "Export complete",
			Badge:  "In Sync",
			Lines:  lines,
			Hint:   "any key to exit",
			Width:  a.width,
			Height: a.height,
		})

	case importStepFailed:
		errText := "unknown error"
		if a.err != nil {
			errText = a.err.Error()
		}
		return views.RenderImportResult(views.ImportResultModel{
			Title:  "Export failed",
			Badge:  "Drift",
			Lines:  []string{errText},
			Hint:   "any key to exit",
			Width:  a.width,
			Height: a.height,
		})
	}
	return ""
}

// fileExistsIn reports whether name exists under dir (or as an absolute path).
func fileExistsIn(dir, name string) bool {
	joined := name
	if dir != "" && !filepath.IsAbs(name) {
		joined = filepath.Join(dir, name)
	}
	info, err := os.Lstat(joined)
	return err == nil && !info.IsDir()
}
