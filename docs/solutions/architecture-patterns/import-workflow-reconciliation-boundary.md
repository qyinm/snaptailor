---
title: Safe agent configuration import and multi-agent reconciliation boundary
date: 2026-09-03
last_updated: 2026-09-03
category: docs/solutions/architecture-patterns
module: Gandalf importer
problem_type: architecture_pattern
component: tooling
severity: high
applies_when:
  - Reverse-generating canonical manifests from heterogeneous agent configuration files
  - Templatizing secrets and credentials into version-controlled team templates
  - Mirroring discovered skills into project-local canonical skill directories
  - Preventing path traversal, symlink redirection, and unintended file overwrite
tags: [importer, security, path-confinement, secret-redaction, skill-mirroring, symlinks, toml-codec]
---

# Safe Agent Configuration Import and Multi-Agent Reconciliation Boundary

## Context

When teams adopt "Agent Environment as Code", individual engineers already have fragmented, local configuration files across Claude Code (`.mcp.json`, `~/.claude.json`), OpenAI Codex (`~/.codex/config.toml`), and Cursor (`.cursor/mcp.json`, `~/.cursor/mcp.json`).

The `gandalf export` command (with `gandalf import` as a backward-compatible alias) was designed to reverse-engineer these existing setups into a single, clean `gandalf.toml` manifest without manual copy-pasting. However, exporting native agent setups into a team manifest poses unique security and reliability risks:
1. Hardcoded API tokens, database connection strings, and OAuth tokens in headers, commands, or arguments can leak into git-tracked files.
2. In-project or destination directory symlinks can allow attacker-manipulated path traversal outside the repository boundary.
3. Overwriting existing team skills or aborting midway through skill copying can leave repository workspaces in corrupted states.

## Key Principles & Solutions

### 1. Two-Tier Path Confinement (Lexical + Symlink Boundary)
Never rely solely on lexical `filepath.Rel(projectRoot, target)` checks:
- An adversary or malicious repository can place symlinks inside `.gandalf` or within the target `--output` directory tree pointing to `/etc/` or `~/.ssh`.
- **Solution**:
  - `verifyDestinationPathConfinement`: Inspect every path component from the project root down to the target with `os.Lstat`. If any ancestor component is a symlink, reject immediately.
  - Evaluate physical paths using `filepath.EvalSymlinks`, properly normalizing platform-specific symlink roots (such as macOS `/var` -> `/private/var`).

### 2. Iterative Multi-Token Secret Redaction
Single-pass regex replacement leaves subsequent credentials intact when an argument or command string contains multiple secrets (e.g. `--db=postgres://... --token=sk-ant-...`):
- **Solution**:
  - Iterative regex replacement loops (`redactStringValue`) for Database URLs, Anthropic keys, OpenAI keys, GitHub tokens, and Bearer tokens.
  - Treat all unrecognized credentials in sensitive headers (`Authorization`, `X-API-Key`, `Token`, `Secret`) as secrets, templatizing them into `${SERVER_HEADER_KEY}` with sanitized non-functional placeholders (`sample-auth-token`) in `[env_template]`.

### 3. Safe Skill Mirroring with Atomic Cleanup
- **Validation**: Only mirror directories that contain a valid `SKILL.md` file, ignoring cache directories, `node_modules`, or temporary folders.
- **Precedence**: Project-scoped skills must always override global-scoped skills of the same name.
- **Protection**: Require `--force` to overwrite existing skills in `.gandalf/skills/<name>`.
- **Atomic Cleanup**: Track newly created skill directories during copying and roll them back (`os.RemoveAll`) if the subsequent manifest write fails.

### 4. Deterministic TOML Codec
- Map iteration in Go is randomized. Iterating server definitions directly causes non-deterministic manifest diffs on identical inputs.
- Always sort keys before invoking templatizers, formatters, or array serializations.
- Escape or quote table header names (e.g. `[mcp_servers."server.with.dots"]`) so that dotted server names are not misparsed as nested TOML subtables.
