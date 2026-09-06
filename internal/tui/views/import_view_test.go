package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func assertLinesFit(t *testing.T, rendered string, width int) {
	t.Helper()
	for i, line := range strings.Split(rendered, "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("line %d exceeds width %d (got %d): %q", i, width, w, ansi.Strip(line))
		}
	}
}

func assertLineCount(t *testing.T, rendered string, maxLines int) {
	t.Helper()
	if got := len(strings.Split(rendered, "\n")); got > maxLines {
		t.Errorf("expected at most %d lines, got %d", maxLines, got)
	}
}

func sampleSelectModel(width, height int) ImportSelectModel {
	longDetail := strings.Repeat("very-long-command-", 12)
	return ImportSelectModel{
		Title:       "Export agent setup",
		ProjectPath: "/Users/someone/projects/with-a-rather-long-path-name/gandalf",
		Width:       width,
		Height:      height,
		Groups: []ImportGroupView{
			{
				Agent: "Claude Code",
				Scope: "project",
				Path:  "/repo/.mcp.json",
				Items: []ImportItemView{
					{Kind: "server", Name: "db-server", Detail: "npx -y @mcp/db", Selected: true, Cursor: true},
					{Kind: "server", Name: "a-server-name-that-is-absurdly-long-and-would-overflow", Detail: longDetail, Selected: false},
				},
			},
			{
				Agent: "Cursor",
				Scope: "global",
				Path:  "~/.cursor/mcp.json",
				Items: []ImportItemView{
					{Kind: "skill", Name: "deploy", Detail: "Deploy the service", Selected: true},
				},
			},
		},
	}
}

func TestRenderImportSelect_FitsStandardTerminal(t *testing.T) {
	rendered := RenderImportSelect(sampleSelectModel(80, 24))
	assertLinesFit(t, rendered, 80)
	assertLineCount(t, rendered, 24)

	plain := ansi.Strip(rendered)
	for _, want := range []string{"Claude Code", "Cursor", "db-server", "[Discovered]", "space toggle", "tab preview", "enter export", "q cancel"} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected select screen to contain %q", want)
		}
	}
}

func TestRenderImportSelect_NarrowTerminalDoesNotOverflow(t *testing.T) {
	for _, width := range []int{60, 40, 24} {
		rendered := RenderImportSelect(sampleSelectModel(width, 24))
		assertLinesFit(t, rendered, width)
	}
}

func TestRenderImportSelect_CheckboxState(t *testing.T) {
	rendered := ansi.Strip(RenderImportSelect(sampleSelectModel(80, 24)))
	if !strings.Contains(rendered, "[x] db-server") {
		t.Errorf("expected selected server to render a checked box")
	}
	if !strings.Contains(rendered, "[ ] a-server-name") {
		t.Errorf("expected unselected server to render an empty box")
	}
}

func TestRenderImportPreview_FitsAndMasks(t *testing.T) {
	var toml strings.Builder
	toml.WriteString("version = \"1.0\"\nname = \"demo\"\n\n[mcp_servers.db]\n")
	toml.WriteString("args = [\"-y\", \"db\", \"" + strings.Repeat("x", 200) + "\"]\n")
	for i := 0; i < 60; i++ {
		toml.WriteString("# filler line to force scrolling\n")
	}

	model := ImportPreviewModel{
		OutputFile:  "gandalf.toml",
		TOML:        toml.String(),
		Offset:      0,
		MaskedCount: 3,
		Width:       80,
		Height:      24,
	}
	rendered := RenderImportPreview(model)
	assertLinesFit(t, rendered, 80)
	assertLineCount(t, rendered, 24)

	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "3 secret(s) masked as ${VAR}") {
		t.Errorf("expected masked-secret note in preview header")
	}
	if !strings.Contains(plain, "+ version") {
		t.Errorf("expected diff-style '+' prefixes on manifest lines")
	}
	if !strings.Contains(plain, "lines 1-") {
		t.Errorf("expected scroll position hint in help bar")
	}

	// Border alignment: pane rows must share the same visible width.
	var paneWidths map[int]bool = map[int]bool{}
	inPane := false
	for _, line := range strings.Split(rendered, "\n") {
		stripped := ansi.Strip(line)
		if strings.HasPrefix(stripped, "╭") {
			inPane = true
		}
		if inPane {
			paneWidths[ansi.StringWidth(line)] = true
		}
		if strings.HasPrefix(stripped, "╰") {
			break
		}
	}
	if len(paneWidths) != 1 {
		t.Errorf("expected all pane border rows to have identical width, got %v", paneWidths)
	}
}

func TestRenderImportPreview_OffsetScrolls(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("a", 10)
	}
	model := ImportPreviewModel{OutputFile: "gandalf.toml", TOML: strings.Join(lines, "\n"), Offset: 10, Width: 80, Height: 24}
	plain := ansi.Strip(RenderImportPreview(model))
	if !strings.Contains(plain, "lines 11-") {
		t.Errorf("expected scroll hint to reflect offset 10, got status line missing")
	}
}

func TestRenderImportResult_Badges(t *testing.T) {
	done := ansi.Strip(RenderImportResult(ImportResultModel{
		Title: "Export complete", Badge: "In Sync",
		Lines: []string{"Generated gandalf.toml"}, Hint: "any key to exit", Width: 80, Height: 24,
	}))
	if !strings.Contains(done, "[In Sync]") {
		t.Errorf("expected [In Sync] badge on done screen")
	}

	failed := ansi.Strip(RenderImportResult(ImportResultModel{
		Title: "Export failed", Badge: "Drift",
		Lines: []string{"boom"}, Hint: "any key to exit", Width: 80, Height: 24,
	}))
	if !strings.Contains(failed, "[Drift]") || !strings.Contains(failed, "boom") {
		t.Errorf("expected [Drift] badge and error text on failed screen")
	}
}

func TestRenderImportSelect_WarningsStayVisible(t *testing.T) {
	model := sampleSelectModel(80, 24)
	model.Warnings = []string{"failed to parse /repo/.cursor/mcp.json: unexpected end of JSON input"}
	plain := ansi.Strip(RenderImportSelect(model))
	if !strings.Contains(plain, "⚠ failed to parse /repo/.cursor/mcp.json") {
		t.Errorf("expected partial-scan warning to remain visible on the select screen")
	}
	assertLinesFit(t, RenderImportSelect(model), 80)
}

func TestRenderImportSelect_CursorRowSurvivesManyWarnings(t *testing.T) {
	model := sampleSelectModel(80, 24)
	// Flood the screen with warnings so the body overflows the viewport.
	for i := 0; i < 30; i++ {
		model.Warnings = append(model.Warnings, strings.Repeat("w", 10)+strings.Repeat("arning ", 3))
	}
	// Add many items; put the cursor on the very last one.
	var extra []ImportItemView
	for i := 0; i < 30; i++ {
		extra = append(extra, ImportItemView{Kind: "server", Name: strings.Repeat("s", 5) + string(rune('a'+i%26)) + strings.Repeat("v", i%3), Selected: true})
	}
	extra = append(extra, ImportItemView{Kind: "server", Name: "cursor-target-server", Selected: true, Cursor: true})
	model.Groups[0].Items = append(model.Groups[0].Items, extra...)

	rendered := RenderImportSelect(model)
	assertLinesFit(t, rendered, 80)
	assertLineCount(t, rendered, 24)
	if !strings.Contains(ansi.Strip(rendered), "cursor-target-server") {
		t.Errorf("cursor row must stay visible even when warnings overflow the viewport")
	}
}

func TestRenderImportLoading_Fits(t *testing.T) {
	rendered := RenderImportLoading("Export agent setup", "/repo", 80, 24)
	assertLinesFit(t, rendered, 80)
	assertLineCount(t, rendered, 24)
	if !strings.Contains(ansi.Strip(rendered), "Scanning") {
		t.Errorf("expected scanning message on loading screen")
	}
}
