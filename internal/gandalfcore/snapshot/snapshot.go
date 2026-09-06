package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/audit"
	"github.com/qyinm/gandalf/internal/gandalfcore/graph"
	"github.com/qyinm/gandalf/internal/gandalfcore/provenance"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	_ "github.com/qyinm/gandalf/internal/gandalfcore/scan/plugins"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

var (
	agentPathPattern        = regexp.MustCompile(`^[a-z_]+:`)
	safeFileNamePattern     = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)
	secretAssignmentPattern = regexp.MustCompile(`(?i)(?:api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)\s*=`)
	secretKeyPattern        = regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|passwd|credential|private[_-]?key|auth|bearer)`)
)

type contentCapture struct {
	evidence []types.DiscoveredItem
	content  []types.SnapshotContentEntry
}

// CaptureCurrentState scans the project, builds analysis artifacts, and returns a snapshot-ready state.
func CaptureCurrentState(options *types.RuntimeOptions, name string) (*types.CurrentState, error) {
	storeFindings, err := store.EnsureStore(options.StoreDir)
	if err != nil {
		return nil, err
	}

	scanOptions := &types.ScanOptions{
		ProjectPath: options.ProjectPath,
		HomeDir:     options.HomeDir,
		StoreDir:    options.StoreDir,
		Agent:       options.Agent,
		Scope:       options.Scope,
	}
	baseScan := scan.ScanProject(scanOptions)

	var capture contentCapture
	if options.CaptureContent {
		captured, err := captureContentBackedEvidence(baseScan.Evidence, options)
		if err != nil {
			return nil, err
		}
		capture = captured
	} else {
		capture = contentCapture{evidence: baseScan.Evidence}
	}

	scanResult := types.ScanResult{
		Trust:      baseScan.Trust,
		Evidence:   capture.evidence,
		BlindSpots: baseScan.BlindSpots,
	}

	graphNodes := graph.BuildGraph(scanResult.Evidence)
	auditFindings := append([]types.AuditFinding{}, storeFindings...)
	auditFindings = append(auditFindings, audit.AuditEvidence(scanResult.Evidence, graphNodes)...)
	provenanceEntries := provenance.BuildProvenance(graphNodes, scanResult.Evidence)

	redactionPolicy := "metadata-only"
	if options.CaptureContent {
		redactionPolicy = "content-backed"
	}

	snapshot := types.Snapshot{
		Manifest: types.SnapshotManifest{
			SchemaVersion: "0.1",
			Name:          name,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
			ProjectPath:   options.ProjectPath,
			Security: types.SnapshotSecurity{
				RawSecretsIncluded: false,
				RedactionPolicy:    redactionPolicy,
			},
		},
		Evidence:      scanResult.Evidence,
		Graph:         graphNodes,
		AuditFindings: auditFindings,
		Provenance:    provenanceEntries,
		Content:       capture.content,
	}

	return &types.CurrentState{
		Scan:          scanResult,
		Snapshot:      snapshot,
		StoreFindings: storeFindings,
	}, nil
}

func captureContentBackedEvidence(evidence []types.DiscoveredItem, options *types.RuntimeOptions) (contentCapture, error) {
	content := make([]types.SnapshotContentEntry, 0)
	byEvidenceID := make(map[string]types.SnapshotContentEntry)

	for i := range evidence {
		item := &evidence[i]
		restorePath, ok := restorePathForContent(item)
		if !ok {
			continue
		}
		if !isContentCaptureCandidate(item) {
			continue
		}
		absolutePath, ok := absolutePathForSourcePath(restorePath, options)
		if !ok {
			continue
		}
		textBytes, err := os.ReadFile(absolutePath)
		if err != nil {
			continue
		}
		text := string(textBytes)
		checksum := sha256Checksum(text)
		storagePath := fmt.Sprintf("content/%s.txt", safeContentFileName(item.ID, checksum))

		var entry types.SnapshotContentEntry
		if containsSecretLikeContent(text) {
			reason := "secret_like_content"
			entry = types.SnapshotContentEntry{
				EvidenceID:    item.ID,
				SourcePath:    item.SourcePath,
				RestorePath:   restorePath,
				Checksum:      checksum,
				ByteLength:    uint64(len(textBytes)),
				Encoding:      "utf8",
				StoragePath:   storagePath,
				CaptureStatus: "omitted",
				Reason:        &reason,
				Content:       nil,
			}
		} else {
			entry = types.SnapshotContentEntry{
				EvidenceID:    item.ID,
				SourcePath:    item.SourcePath,
				RestorePath:   restorePath,
				Checksum:      checksum,
				ByteLength:    uint64(len(textBytes)),
				Encoding:      "utf8",
				StoragePath:   storagePath,
				CaptureStatus: "captured",
				Reason:        nil,
				Content:       &text,
			}
		}

		byEvidenceID[item.ID] = entry
		content = append(content, entry)
	}

	updatedEvidence := make([]types.DiscoveredItem, len(evidence))
	for i := range evidence {
		item := evidence[i]
		entry, ok := byEvidenceID[item.ID]
		if !ok {
			updatedEvidence[i] = item
			continue
		}

		metadata := map[string]any{}
		if len(item.Metadata) > 0 {
			_ = json.Unmarshal(item.Metadata, &metadata)
		}
		metadata["contentCaptureStatus"] = entry.CaptureStatus
		metadata["contentRestorePath"] = entry.RestorePath
		if entry.Reason != nil {
			metadata["contentCaptureReason"] = *entry.Reason
		}
		metaBytes, _ := json.Marshal(metadata)
		checksum := entry.Checksum
		updatedEvidence[i] = types.DiscoveredItem{
			ID:            item.ID,
			Agent:         item.Agent,
			Kind:          item.Kind,
			SourcePath:    item.SourcePath,
			Scope:         item.Scope,
			Precedence:    item.Precedence,
			Parser:        item.Parser,
			Sensitivity:   item.Sensitivity,
			ContentPolicy: item.ContentPolicy,
			RestorePolicy: item.RestorePolicy,
			CaptureStatus: item.CaptureStatus,
			Confidence:    item.Confidence,
			Name:          item.Name,
			Value:         item.Value,
			Checksum:      &checksum,
			Metadata:      metaBytes,
		}
	}

	return contentCapture{evidence: updatedEvidence, content: content}, nil
}

func isContentCaptureCandidate(item *types.DiscoveredItem) bool {
	if isProjectMCPContentCandidate(item) {
		return true
	}
	return isUserGlobalContentCandidate(item)
}

func isProjectMCPContentCandidate(item *types.DiscoveredItem) bool {
	if item.Scope != types.ScopeProject {
		return false
	}
	if item.CaptureStatus != types.CaptureCaptured {
		return false
	}
	if item.Kind != types.KindAgentConfig {
		return false
	}
	switch filepath.ToSlash(item.SourcePath) {
	case ".mcp.json", ".cursor/mcp.json", ".codex/config.toml":
		return true
	default:
		return false
	}
}

func isUserGlobalContentCandidate(item *types.DiscoveredItem) bool {
	if item.Scope != types.ScopeUser {
		return false
	}
	if item.CaptureStatus != types.CaptureCaptured {
		return false
	}
	if !strings.HasPrefix(item.SourcePath, "~/") {
		return false
	}
	if item.SourcePath == "~/.claude.json" {
		return false
	}
	if item.Kind != types.KindAgentConfig && item.Kind != types.KindSkill && item.Kind != types.KindHook {
		return false
	}
	return userGlobalPathForAgent(item.Agent, item.SourcePath)
}

func userGlobalPathForAgent(agent types.AgentID, sourcePath string) bool {
	switch agent {
	case types.AgentCodex:
		return strings.HasPrefix(sourcePath, "~/.codex/")
	case types.AgentClaudeCode:
		return strings.HasPrefix(sourcePath, "~/.claude/")
	case types.AgentCursor:
		return strings.HasPrefix(sourcePath, "~/.cursor/") || strings.HasPrefix(sourcePath, "~/.agents/")
	case types.AgentOpencode:
		return strings.HasPrefix(sourcePath, "~/.config/opencode/") ||
			strings.HasPrefix(sourcePath, "~/.claude/skills/") ||
			strings.HasPrefix(sourcePath, "~/.codex/skills/")
	case types.AgentPiAgent:
		return strings.HasPrefix(sourcePath, "~/.pi/") || strings.HasPrefix(sourcePath, "~/.agents/")
	default:
		return false
	}
}

func restorePathForContent(item *types.DiscoveredItem) (string, bool) {
	if item.Kind == types.KindSkill {
		entrypoint := "SKILL.md"
		if len(item.Metadata) > 0 {
			var meta map[string]json.RawMessage
			if json.Unmarshal(item.Metadata, &meta) == nil {
				if v, ok := meta["entrypoint"]; ok {
					if s := jsonString(v); s != "" {
						entrypoint = s
					}
				}
			}
		}
		return item.SourcePath + "/" + entrypoint, true
	}
	return item.SourcePath, true
}

func absolutePathForSourcePath(sourcePath string, options *types.RuntimeOptions) (string, bool) {
	if sourcePath == "~" {
		return options.HomeDir, true
	}
	if rest, ok := strings.CutPrefix(sourcePath, "~/"); ok {
		return filepath.Join(options.HomeDir, rest), true
	}
	if filepath.IsAbs(sourcePath) {
		return sourcePath, true
	}
	if agentPathPattern.MatchString(sourcePath) {
		return "", false
	}
	joined := filepath.Join(options.ProjectPath, sourcePath)
	if resolved, err := filepath.EvalSymlinks(joined); err == nil {
		return resolved, true
	}
	return joined, true
}

func safeContentFileName(evidenceID, checksum string) string {
	suffix := strings.TrimPrefix(checksum, "sha256:")
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	name := evidenceID + "-" + suffix
	name = safeFileNamePattern.ReplaceAllString(name, ".")
	return strings.Trim(strings.ToLower(name), ".")
}

func containsSecretLikeContent(text string) bool {
	if secretAssignmentPattern.MatchString(text) {
		return true
	}
	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return false
	}
	return jsonValueContainsSecretLikeKey(parsed)
}

func jsonValueContainsSecretLikeKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if secretKeyPattern.MatchString(key) {
				return true
			}
			if jsonValueContainsSecretLikeKey(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if jsonValueContainsSecretLikeKey(nested) {
				return true
			}
		}
	}
	return false
}

func sha256Checksum(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func jsonString(value json.RawMessage) string {
	var s string
	if json.Unmarshal(value, &s) == nil {
		return s
	}
	return ""
}
