---
title: Pair check and apply on the same project-only MCP scope
date: 2026-09-05
last_updated: 2026-09-05
category: docs/solutions/architecture-patterns
module: Gandalf sync
problem_type: architecture_pattern
component: tooling
severity: high
applies_when:
  - Changing gandalf check --project-only or the packaged GitHub Action
  - Changing gandalf apply write targets or Smart Merge destinations
  - Reporting project MCP drift or telling users how to remediate CI failures
tags: [check, apply, project-only, ci-drift-gate, mcp, smart-merge]
---

# Pair check and apply on the same project-only MCP scope

## Context

`gandalf check --project-only` and the GitHub Action compare `gandalf.toml` with repository agent files (`.mcp.json`, `.cursor/mcp.json`, `.codex/config.toml`). Default `gandalf apply` Smart-Merges user-home configs only. Telling CI failures to run `gandalf apply` is a false remediation: home writes cannot clear project-scoped drift.

## Key Principles & Solutions

### 1. Same flag, same write/read targets
`--project-only` on check and apply must name the same files. Do not make CI fail on a target default apply refuses to write.

### 2. Do not weaken DetectProjectDrift to hide the gap
Project MCP comparison is the CI contract. The fix is a matching apply path, not skipping `DriftUnsyncedProjectConfig`.

### 3. Keep project writes uninterpolated
CI loads the manifest with `NoInterpolate`. Project apply must do the same so `${ENV}` stays in git and process secrets never land in committed agent files.

### 4. Home apply stays home-only
Default apply remains the developer-machine Smart Merge. Creating project files is opt-in (`--project-only`) so a local sync does not dirty the repository by surprise.

### 5. Project apply is a reviewed reconcile, not a stale write
Review Changes is not apply authority. Re-read each project target at apply time and re-merge if it changed, so concurrent unrelated keys survive. Pre-apply snapshots use project scope and must capture those files before a rollback name is reported.

### 6. Extra project MCP servers are drift that apply closes
CI rejects undeclared servers. Project apply therefore drops unmanaged `mcpServers` / `[mcp_servers.*]` entries while preserving non-server keys. Home Smart Merge still keeps personal servers.
