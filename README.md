# shh-h

`shh-h` is a modular Go desktop terminal and SSH toolbox in the style of
MobaXterm. It packages the Go backend and React frontend as a self-contained
native desktop application.

## What Works

- A Wails v2 native host with an embedded React and strict TypeScript frontend.
- Real interactive local shells through a pseudoterminal on macOS and Linux.
- xterm.js rendering, input, resize, search, titles, bells, and persistent tabs.
- Searchable local and SSH profiles with create, edit, duplicate, delete,
  grouping, tags, favorites, versioned atomic persistence, private JSON export,
  and strict atomic import from shh-h JSON or concrete OpenSSH hosts.
- One-off SSH quick connect with transient validated targets and the same strict
  host-key verification and credential handling as saved profiles.
- Interactive SSH terminals with strict known-host verification, explicit
  first-use trust, agent, key, password, and keyboard-interactive authentication.
- An SFTP browser with streamed upload/download, bounded concurrency, progress,
  cancellation, atomic partial files, directory operations, and permissions.
- Saved local, remote, and dynamic SOCKS5 SSH tunnels with explicit lifecycle,
  actual bound addresses, retry policy, auto-start policy, and loopback defaults.
- Saved command snippets with strict variables, backend-rendered previews, and
  guarded execution across explicitly selected live terminals.
- Opt-in per-session output logging with optional line timestamps, private file
  permissions, bounded rotation, visible state, and terminal-owned cleanup.
- Versioned terminal settings for font, spacing, cursor, scrollback, and bell
  behavior, with live application to open terminals and durable reset support.
- A searchable command palette with grouped actions, disabled-state feedback,
  keyboard navigation, and shell-safe application shortcuts.
- Saved workspace layouts with private atomic persistence, ordered profile-tab
  snapshots, disconnected restoration, and explicit per-tab reconnection.
- Explicit session ownership, activation, ordered input/output, bounded output
  flow control, frontend leases, and deterministic shutdown of terminals,
  transfers, SFTP clients, and tunnels.
- Single-instance handling and confirmation before closing live resources.
- A single macOS application bundle with no web server, Node runtime, daemon,
  or sidecar process required at runtime.

The current native proof and packaged build target is macOS arm64. Windows
ConPTY, signed/notarized release automation, SSH connection pooling, resumable
transfers, connection and transfer preferences, notifications, and remaining
cross-platform UX are still future milestones; see the implementation plan for
the release gates.

## Prerequisites

- Go 1.26.5 or newer.
- Node.js 24.18 or newer and npm 11 for frontend development.
- The repository-local Wails CLI at `bin/wails`.

Node and npm are build-time dependencies only.

## Commands

```sh
make run       # Wails development mode
make test      # Go and frontend unit/integration tests
make lint      # ESLint and go vet
make build     # production native package
```

On macOS, `make build` produces `build/bin/shh-h.app`. The embedded Go
executable is `build/bin/shh-h.app/Contents/MacOS/shhh`.

## Structure

- `cmd/shhh`: executable entry point and Wails project configuration.
- `internal/app`: composition root and desktop lifecycle configuration.
- `internal/domain`: pure profile, transfer, SSH, tunnel, snippet, workspace,
  and settings models.
- `internal/port`: terminal, SSH connection, and remote filesystem contracts.
- `internal/adapter`: PTY, SSH, known-host, SFTP, session-log, profile exchange, and configuration adapters.
- `internal/usecase`: profile, session, transfer, SSH, tunnel, snippet, and
  workspace orchestration.
- `internal/bridge`: narrow typed Wails command and event boundary.
- `frontend`: React terminal, profile, file, transfer, tunnel, snippet, layout,
  and settings workspaces.
- `docs/IMPLEMENTATION_PLAN.md`: milestones, acceptance criteria, and release
  scope.
- `docs/REMOTE_PROJECTS_PLAN.md`: proposed self-hosted remote code editor,
  provisioning, project lifecycle, and browser-authentication design.
- `docs/adr/0001-desktop-frontend-stack.md`: accepted frontend decision and
  tradeoffs.

The dependency direction and runtime ownership rules are documented in
`docs/ARCHITECTURE.md`.
