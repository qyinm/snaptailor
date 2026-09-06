# Gandalf Architecture

Gandalf is the **Infrastructure as Code (IaC) layer and local control console for AI agent environments** (Claude Code, OpenAI Codex, Cursor). It allows teams to declare, verify, and synchronize agent capabilities (MCP servers, skills, guardrail hooks, and environment templates) via a version-controlled `gandalf.toml` manifest, with non-destructive Smart Merge, automated safety snapshots, and CI drift gating.

---

## 1. System Shape & Monorepo Architecture

The repository is structured as a **Polyglot Monorepo orchestrated by Turborepo and Bun Workspaces**:

```text
gandalf/
├── cmd/gandalf              # CLI Entrypoint (Go)
├── internal/
│   ├── cli/                 # Subcommand wiring (init, check, apply, restore, etc.)
│   ├── gandalfcore/         # Core IaC & Safety Engine
│   │   ├── manifest/        # gandalf.toml parser & schema validator
│   │   ├── sync/            # Non-destructive Smart Merge engine
│   │   ├── scan/            # Read-only agent discovery (Claude Code, Codex, Cursor)
│   │   ├── snapshot/        # SHA-256 pre-apply snapshot creator
│   │   ├── restore/         # Rollback & safety restore planner
│   │   └── pathconfinement/ # Strict root containment boundary
│   └── tui/                 # Interactive Terminal UI (Bubble Tea)
│
├── apps/
│   ├── landing/             # Web Landing Page (Astro 5 + React 19 + xterm.js)
│   └── docs/                # Mintlify Documentation (docs.usegandalf.com)
│
├── packages/
│   └── config-typescript/   # Shared TypeScript base configuration
│
├── turbo.json               # Turborepo task pipeline (build, test, typecheck)
├── package.json             # Monorepo root scripts & Bun workspaces
└── Makefile                 # Standalone Go build & regression target runner
```

---

## 2. Core Operational Pillars (IaC Lifecycle)

Gandalf enforces a 3-step declarative IaC workflow:

```text
[ 1. Declare ]         [ 2. Plan / Drift Gate ]       [ 3. Apply / Restore ]
  gandalf init             gandalf check [--ci]          gandalf apply / restore
       │                            │                               │
       ▼                            ▼                               ▼
  gandalf.toml          Compare local state vs          Pre-apply snapshot (SHA-256)
  (Git-tracked)         manifest. Exit 1 on drift       + Non-destructive Smart Merge
```

### 1. `gandalf init` (Manifest Declaration)
* Scans local active agents (Claude Code, Codex, Cursor) and generates a standardized, human-readable `gandalf.toml` manifest.
* Declares required MCP servers, skills, guardrail hooks, and environment variable templates.

### 2. `gandalf check [--ci]` (Drift Gatekeeper)
* Evaluates current machine/repo configuration against the canonical `gandalf.toml`.
* Detects missing servers, misconfigured arguments, or unregistered skills.
* In CI mode (`--ci`), returns exit code `0` on parity and `1` on drift, enabling automated PR merge blocking in GitHub Actions.

### 3. `gandalf apply` & `gandalf restore` (Safe Synchronization)
* **Pre-apply Safety Snapshot**: Automatically creates a content-addressed SHA-256 snapshot of all target config files before modifying any file.
* **Non-destructive Smart Merge**: Merges manifest-owned keys into agent configurations (e.g. `settings.json`, `config.toml`) while preserving individual developer custom keys and comments.
* **One-command Rollback (`gandalf restore`)**: Instantly reverts the machine to the pre-apply snapshot state if needed.

---

## 3. Core Engine Subsystems (`internal/gandalfcore`)

| Package | Responsibility |
| :--- | :--- |
| `manifest/` | Parses and validates `gandalf.toml` schemas and environment variable bindings. |
| `sync/` | Executes structural Smart Merge against agent-native config files (JSON/TOML). |
| `scan/` & `scan/plugins/` | Inspects Claude Code (`~/.claude.json`, `settings.json`, project `.mcp.json`), Codex (`~/.codex/config.toml`, project `.codex/config.toml`), and Cursor (`.cursor/` / `~/.cursor/`, including Agent CLI config) without executing tools. |
| `snapshot/` & `store/` | Generates immutable, compressed SHA-256 snapshots with metadata and timestamps. |
| `restore/` | Compares baseline snapshots against current state and orchestrates safe rollback. |
| `pathconfinement/` | Enforces strict path security, refusing to read or write outside designated project/home roots and denying symlinks. |
| `policy/` | Evaluates permission boundaries, required environment variables, and execution safety rules. |

---

## 4. Trust Contract & Security Boundaries

* **Read-only Scan Rule**: Scanners inspect static configuration files only. They never execute arbitrary MCP commands, shell hooks, or network requests during discovery.
* **Explicit Write Boundary**: File writes only occur during explicit `gandalf apply` or `gandalf restore` executions.
* **Path Confinement**: All writes are strictly confined to known configuration locations (`$HOME/.claude/`, `$HOME/.codex/`, `$HOME/.cursor/`, and local project directory). Symlink write targets are rejected.
* **100% Local-First**: Gandalf operates entirely offline with zero external SaaS dependencies or telemetry leaks.

---

## 5. Distribution & CI/CD Posture

* **Binary Distribution**:
  * Homebrew: `brew install qyinm/tap/gandalf` (automated via GoReleaser on `v*` tags).
  * Direct script: `curl -fsSL https://usegandalf.com/install.sh | bash`.
  * Source: `go build -o bin/gandalf ./cmd/gandalf` or `bun run build`.
* **CI Validation (`.github/workflows/ci.yml`)**:
  * Unified Turborepo task: `bun run typecheck`, `bun run build`, and `bun run test`.
  * Go Unit & Acceptance Tests: `go test -count=1 ./...`, `./scripts/restore-safety-regression.sh`, `./scripts/gate2-console-acceptance.sh`.
