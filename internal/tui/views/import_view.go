package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ImportFocusPane indicates which panel currently has keyboard focus.
type ImportFocusPane int

const (
	FocusItems ImportFocusPane = iota
	FocusPreview
)

// ImportItemView is one toggleable server or skill row in the import wizard.
type ImportItemView struct {
	Kind          string // "server" | "skill"
	Name          string
	Detail        string // e.g. server command or skill source
	Selected      bool
	Cursor        bool
	Status        string   // "In Sync", "Drift", "Discovered"
	TemplatedEnvs []string // e.g. ["DATABASE_URL"]
}

// ImportGroupView groups discovered items under one source agent.
type ImportGroupView struct {
	Agent string // display name, e.g. "Claude Code", "Cursor", "Codex"
	Scope string // "project" | "global"
	Path  string
	Items []ImportItemView
}

// ImportViewModel is the render input for the unified two-column import view.
type ImportViewModel struct {
	Title            string
	ProjectPath      string
	Groups           []ImportGroupView
	Warnings         []string
	OutputFile       string
	PreviewTOML      string
	PreviewOffset    int
	MaskedCount      int
	ActivePane       ImportFocusPane
	ConfirmOverwrite bool
	Width            int
	Height           int
}

// ImportSelectModel is the legacy render input for the import selection screen.
type ImportSelectModel struct {
	Title       string
	ProjectPath string
	Groups      []ImportGroupView
	Warnings    []string // partial-scan failures that must stay visible
	Width       int
	Height      int
}

// ImportPreviewModel is the render input for the manifest preview screen.
type ImportPreviewModel struct {
	OutputFile  string
	TOML        string
	Offset      int
	MaskedCount int
	Width       int
	Height      int
	Active      bool
}

// ImportResultModel is the render input for done / cancelled / failed states.
type ImportResultModel struct {
	Title  string
	Badge  string // "In Sync" | "Drift" | "Discovered"
	Lines  []string
	Hint   string
	Width  int
	Height int
}

// ImportBadge renders a high-contrast status badge like [In Sync].
func ImportBadge(label string) string {
	switch label {
	case "In Sync":
		return cleanStyle.Render("[" + label + "]")
	case "Drift":
		return changedStyle.Render("[" + label + "]")
	case "Discovered":
		return focusStyle.Render("[" + label + "]")
	default:
		return labelStyle.Render("[" + label + "]")
	}
}

// importHelpBar renders the keybinding help footer.
func importHelpBar(width int, entries ...string) string {
	return mutedStyle.Render(truncate(strings.Join(entries, "  ·  "), width))
}

// RenderImportView renders the interactive import screen with responsive
// two-column layout on wide terminals (width >= 88) or stacked on narrower
// displays (width < 88).
func RenderImportView(m ImportViewModel) string {
	width := max(m.Width, 20)
	height := max(m.Height, 10)

	title := m.Title
	if title == "" {
		title = "Gandalf Export Wizard"
	}
	header := RenderHeader(HeaderView{
		Title: title,
		Scope: m.ProjectPath,
		Chips: []HeaderChip{{
			AgentMarker: "export",
			State:       "clean",
			Detail:      "scan",
		}},
	}, width)

	headerLines := len(strings.Split(header, "\n"))
	usedLines := headerLines + 2 // header + divider + status bar
	availableBodyHeight := max(height-usedLines, 6)

	var body string
	if width >= 88 {
		// Two-column side-by-side layout
		leftWidth := width / 2
		rightWidth := width - leftWidth
		leftBox := renderSourcesTreeBox(m.Groups, m.Warnings, leftWidth, availableBodyHeight, m.ActivePane == FocusItems)
		rightBox := renderDiffPreviewBox(m.PreviewTOML, m.OutputFile, m.PreviewOffset, m.MaskedCount, rightWidth, availableBodyHeight, m.ActivePane == FocusPreview)
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	} else {
		// Stacked layout for compact terminals (< 88 columns)
		topHeight := max(availableBodyHeight/2, 4)
		bottomHeight := max(availableBodyHeight-topHeight, 4)
		topBox := renderSourcesTreeBox(m.Groups, m.Warnings, width, topHeight, m.ActivePane == FocusItems)
		bottomBox := renderDiffPreviewBox(m.PreviewTOML, m.OutputFile, m.PreviewOffset, m.MaskedCount, width, bottomHeight, m.ActivePane == FocusPreview)
		body = lipgloss.JoinVertical(lipgloss.Left, topBox, bottomBox)
	}

	var status string
	if m.ConfirmOverwrite {
		status = importHelpBar(width, "Enter / y confirm overwrite", "Esc / n cancel")
	} else if m.ActivePane == FocusPreview {
		status = importHelpBar(width, "j/k scroll preview", "tab items tree", "enter export", "esc back", "q cancel")
	} else {
		status = importHelpBar(width, "space toggle", "↑/↓ nav", "tab diff preview", "enter export", "q cancel")
	}

	frame := RenderFrame(header, body, status, width, height)
	if m.ConfirmOverwrite {
		frame = renderConfirmOverwriteModal(frame, m.OutputFile, width, height)
	}
	return frame
}

// renderSourcesTreeBox renders the left-hand pane containing discovered sources.
func renderSourcesTreeBox(groups []ImportGroupView, warnings []string, width, height int, active bool) string {
	boxWidth := max(width, 10)
	innerWidth := max(boxWidth-2, 1)
	innerHeight := max(height-2, 1)

	border := paneBorder
	titleStyled := titleStyle.Render("Discovered Sources")
	if active {
		border = paneBorder.BorderForeground(colorBrand)
		titleStyled = activeStyle.Render("● Sources & Skills")
	} else {
		border = paneBorder.BorderForeground(colorBorder)
		titleStyled = labelStyle.Render("Sources & Skills")
	}

	var lines []string
	lines = append(lines, truncate(titleStyled, innerWidth))

	for _, warning := range warnings {
		lines = append(lines, truncate(warnStyle.Render("⚠ "+warning), innerWidth))
	}
	if len(warnings) > 0 {
		lines = append(lines, "")
	}

	cursorRow := -1
	for _, group := range groups {
		scope := ""
		if group.Scope != "" {
			scope = " (" + group.Scope + ")"
		}
		head := labelStyle.Render("▼ "+group.Agent+scope) + " " + mutedStyle.Render(group.Path)
		lines = append(lines, truncate(head, innerWidth))
		if len(group.Items) == 0 {
			lines = append(lines, truncate("    "+mutedStyle.Render("no items to export"), innerWidth))
		}
		for _, item := range group.Items {
			if item.Cursor {
				cursorRow = len(lines)
			}
			lines = append(lines, truncate(renderImportItem(item, innerWidth-1), innerWidth))
		}
		lines = append(lines, "")
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Content scrolling to keep cursor visible within innerHeight
	contentHeight := innerHeight
	if len(lines) > contentHeight {
		offset := 0
		if cursorRow >= contentHeight {
			offset = cursorRow - contentHeight + 1
		}
		end := min(offset+contentHeight, len(lines))
		lines = lines[offset:end]
	}
	for len(lines) < contentHeight {
		lines = append(lines, "")
	}

	return border.Width(innerWidth).Render(strings.Join(lines, "\n"))
}

// renderDiffPreviewBox renders the right-hand preview panel.
func renderDiffPreviewBox(toml, outputFile string, offset, maskedCount, width, height int, active bool) string {
	boxWidth := max(width, 10)
	innerWidth := max(boxWidth-2, 1)
	innerHeight := max(height-2, 1)

	border := paneBorder
	titleStyled := titleStyle.Render("gandalf.toml Preview")
	if active {
		border = paneBorder.BorderForeground(colorBrand)
		titleStyled = activeStyle.Render("● gandalf.toml Preview")
	} else {
		border = paneBorder.BorderForeground(colorBorder)
		titleStyled = labelStyle.Render("gandalf.toml Preview")
	}

	if outputFile != "" {
		titleStyled += mutedStyle.Render(" (" + outputFile + ")")
	}
	if maskedCount > 0 {
		titleStyled += cleanStyle.Render(fmt.Sprintf(" 🔒%d", maskedCount))
	}

	rawLines := strings.Split(strings.TrimRight(toml, "\n"), "\n")
	if len(rawLines) == 0 || (len(rawLines) == 1 && rawLines[0] == "") {
		rawLines = []string{"# No items selected for manifest emission"}
	}

	if offset > len(rawLines)-1 {
		offset = max(len(rawLines)-1, 0)
	}
	if offset < 0 {
		offset = 0
	}

	usableLines := max(innerHeight-2, 1) // 1 title line, 1 footer line
	end := min(offset+usableLines, len(rawLines))

	rendered := make([]string, 0, innerHeight)
	rendered = append(rendered, truncate(titleStyled, innerWidth))

	for _, line := range rawLines[offset:end] {
		trimmed := strings.TrimSpace(line)
		var formatted string
		if strings.HasPrefix(trimmed, "#") {
			formatted = mutedStyle.Render(truncate(line, innerWidth))
		} else if strings.HasPrefix(trimmed, "[") {
			formatted = activeStyle.Render(truncate(line, innerWidth))
		} else if trimmed != "" {
			prefix := cleanStyle.Render("+ ")
			maxContent := max(innerWidth-2, 1)
			formatted = prefix + truncate(line, maxContent)
		} else {
			formatted = ""
		}
		rendered = append(rendered, truncate(formatted, innerWidth))
	}
	for len(rendered) < innerHeight-1 {
		rendered = append(rendered, "")
	}

	scrollHint := fmt.Sprintf("lines %d-%d of %d", offset+1, max(end, 1), len(rawLines))
	rendered = append(rendered, truncate(mutedStyle.Render(scrollHint), innerWidth))

	return border.Width(innerWidth).Render(strings.Join(rendered, "\n"))
}

// renderConfirmOverwriteModal renders a centered overlay alert.
func renderConfirmOverwriteModal(background string, outputFile string, width, height int) string {
	modalWidth := min(58, max(width-4, 20))
	contentWidth := max(modalWidth-4, 10)
	lines := []string{
		warnStyle.Bold(true).Render(truncate("⚠️  "+outputFile+" Already Exists", contentWidth)),
		"",
		truncate("Overwrite existing manifest with exported config?", contentWidth),
		truncate("Existing content will be replaced atomically.", contentWidth),
		"",
		activeStyle.Render(truncate("Enter / y Confirm  ·  Esc / n Cancel", contentWidth)),
	}

	box := lipgloss.NewStyle().
		Width(contentWidth).
		Background(colorOverlayBg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorChanged).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))

	bgLines := strings.Split(background, "\n")
	for len(bgLines) < height {
		bgLines = append(bgLines, "")
	}
	boxLines := strings.Split(box, "\n")
	top := max(0, (height-len(boxLines))/2)
	left := max(0, (width-modalWidth)/2)
	for i, boxLine := range boxLines {
		target := top + i
		if target < 0 || target >= len(bgLines) {
			continue
		}
		bgLines[target] = overlayLine(bgLines[target], boxLine, left, width)
	}
	return strings.Join(bgLines, "\n")
}

// RenderImportDiffPreview renders the prospective gandalf.toml with masked secrets
// inside a bordered, scrollable pane.
func RenderImportDiffPreview(m ImportPreviewModel) string {
	width := max(m.Width, 20)
	header := titleStyle.Render("Import preview") + mutedStyle.Render("  "+m.OutputFile)
	if m.MaskedCount > 0 {
		header += cleanStyle.Render(fmt.Sprintf("  %d secret(s) masked as ${VAR}", m.MaskedCount))
	}
	header = truncate(header, width)

	innerWidth := max(width-6, 10) // border(2) + padding(2) + "+ "(2)
	lines := strings.Split(strings.TrimRight(m.TOML, "\n"), "\n")
	bodyHeight := max(m.Height-4, 1) // header + divider + status
	paneHeight := max(bodyHeight-2, 1)

	if m.Offset > len(lines)-1 {
		m.Offset = max(len(lines)-1, 0)
	}
	end := min(m.Offset+paneHeight, len(lines))

	rendered := make([]string, 0, paneHeight)
	for _, line := range lines[m.Offset:end] {
		rendered = append(rendered, truncate(cleanStyle.Render("+ ")+truncate(line, innerWidth), innerWidth+2))
	}
	for len(rendered) < paneHeight {
		rendered = append(rendered, "")
	}

	border := paneBorder
	if m.Active {
		border = paneBorder.BorderForeground(colorBrand)
	}
	pane := border.Width(width - 4).Render(strings.Join(rendered, "\n"))

	scrollHint := fmt.Sprintf("lines %d-%d of %d", m.Offset+1, max(end, 1), len(lines))
	status := importHelpBar(width, "j/k scroll", "tab back", "enter export", "q cancel", scrollHint)
	return RenderFrame(header, pane, status, width, m.Height)
}

// RenderImportPreview is kept for backward compatibility.
func RenderImportPreview(m ImportPreviewModel) string {
	return RenderImportDiffPreview(m)
}

// RenderImportSelect is kept for backward compatibility with existing tests.
func RenderImportSelect(m ImportSelectModel) string {
	width := max(m.Width, 20)
	header := RenderHeader(HeaderView{
		Title: m.Title,
		Scope: m.ProjectPath,
		Chips: []HeaderChip{{
			AgentMarker: "export",
			State:       "clean",
			Detail:      "scan",
		}},
	}, width)

	var body []string
	for _, warning := range m.Warnings {
		body = append(body, truncate(warnStyle.Render("⚠ "+warning), width))
	}
	if len(m.Warnings) > 0 {
		body = append(body, "")
	}
	cursorRow := -1
	for _, group := range m.Groups {
		scope := ""
		if group.Scope != "" {
			scope = " (" + group.Scope + ")"
		}
		head := labelStyle.Render(group.Agent+scope) + "  " + ImportBadge("Discovered") + "  " + mutedStyle.Render(group.Path)
		body = append(body, truncate(head, width))
		if len(group.Items) == 0 {
			body = append(body, truncate("    "+mutedStyle.Render("no items to export"), width))
		}
		for _, item := range group.Items {
			if item.Cursor {
				cursorRow = len(body)
			}
			body = append(body, truncate(renderImportItem(item, width-2), width))
		}
		body = append(body, "")
	}
	if len(body) > 0 {
		body = body[:len(body)-1]
	}

	headerLines := len(strings.Split(header, "\n"))
	avail := max(m.Height-headerLines-2, 1) // header + divider + status
	if len(body) > avail {
		offset := 0
		if cursorRow >= avail {
			offset = cursorRow - avail + 1
		}
		end := min(offset+avail, len(body))
		body = body[offset:end]
	}

	status := importHelpBar(width, "space toggle", "tab preview", "enter export", "q cancel")
	return RenderFrame(header, strings.Join(body, "\n"), status, width, m.Height)
}

func renderImportItem(item ImportItemView, width int) string {
	checkbox := "[ ]"
	if item.Selected {
		checkbox = cleanStyle.Render("[x]")
	}
	kind := mutedStyle.Render(item.Kind)

	statusBadge := ""
	if item.Status != "" {
		statusBadge = " " + ImportBadge(item.Status)
	}

	line := fmt.Sprintf("  %s %s  %s%s", checkbox, item.Name, kind, statusBadge)
	if len(item.TemplatedEnvs) > 0 {
		line += " " + warnStyle.Render("🔒 "+strings.Join(item.TemplatedEnvs, ","))
	} else if item.Detail != "" {
		line += mutedStyle.Render("  " + item.Detail)
	}
	if item.Cursor {
		plain := fmt.Sprintf("  %s %s  %s", "[ ]", item.Name, item.Kind)
		if item.Selected {
			plain = fmt.Sprintf("  %s %s  %s", "[x]", item.Name, item.Kind)
		}
		if len(item.TemplatedEnvs) > 0 {
			plain += " 🔒 " + strings.Join(item.TemplatedEnvs, ",")
		} else if item.Detail != "" {
			plain += "  " + item.Detail
		}
		return selectedRow.Render(truncate("❯"+plain[1:], width))
	}
	return line
}

// RenderImportResult renders terminal states (done, cancelled, failed).
func RenderImportResult(m ImportResultModel) string {
	width := max(m.Width, 20)
	header := RenderHeader(HeaderView{Title: m.Title, Scope: ""}, width)

	var body []string
	if m.Badge != "" {
		body = append(body, ImportBadge(m.Badge), "")
	}
	for _, line := range m.Lines {
		body = append(body, truncate(line, width))
	}
	status := importHelpBar(width, m.Hint)
	return RenderFrame(header, strings.Join(body, "\n"), status, width, m.Height)
}

// RenderImportLoading renders the initial scan-in-progress screen.
func RenderImportLoading(title, projectPath string, width, height int) string {
	width = max(width, 20)
	header := RenderHeader(HeaderView{Title: title, Scope: projectPath}, width)
	body := mutedStyle.Render("Scanning Claude Code, Cursor, and Codex configurations…")
	return RenderFrame(header, body, importHelpBar(width, "q cancel"), width, height)
}
