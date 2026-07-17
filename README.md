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
  cancellation, atomic partial files, directory operations, permissions, and
  profile-scoped remote-path favorites.
- Saved local, remote, and dynamic SOCKS5 SSH tunnels with explicit lifecycle,
  actual bound addresses, retry policy, auto-start policy, and loopback defaults.
- Saved command snippets with strict variables, backend-rendered previews, and
  guarded execution across explicitly selected live terminals.
- Opt-in per-session output logging with optional line timestamps, private file
  permissions, bounded rotation, visible state, and terminal-owned cleanup.
- Versioned terminal settings for font, spacing, cursor, scrollback, and bell
  behavior, with live application to open terminals and durable reset support.
- Opt-in native notifications for long completed transfers and failed terminal
  sessions, with explicit OS permission, category controls, and a test action.
- A searchable command palette with grouped actions, disabled-state feedback,
  keyboard navigation, and shell-safe application shortcuts.
- Terminal viewport copy and exact selection export through native clipboard
  and Save dialogs, with bounded private atomic text files.
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
transfers, connection and transfer preferences, and remaining
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

## Terminal Text Actions

`Copy visible terminal` copies only the active viewport, joins xterm soft wraps,
and ignores right-side cell padding. `Export terminal selection` is enabled only
while the active terminal has selected text and writes that exact selection to
a user-chosen `.txt` or `.log` file. Exports are capped at 16 MiB, use private
permissions, and are atomically replaced; cancelling the native dialog writes
nothing. Both actions are available from the terminal toolbar and command
palette.

## Remote Path Favorites

Inside an open SFTP workspace, use the star beside the path field to add or
remove the current directory. The adjacent Favorites menu navigates to saved
paths for that profile. Favorites are canonical absolute remote paths stored in
a separate private atomic file; loading them never opens a connection, and
choosing one uses only the SFTP session that is already visible.

## System Notifications

Notifications are disabled by default. Enable them under Settings, grant the
operating-system permission through the explicit Allow notifications action,
and choose whether to report completed long-running transfers, unexpected
terminal failures, or both. Upload and download notifications use the saved
duration threshold; short transfers do not produce alerts. The Send test action
is available after notification preferences are saved.

The application never requests permission at startup. Explicitly closed
terminals do not produce disconnect alerts, and transfer alerts include only a
basename, direction, and duration rather than a full local or remote path.
Credentials, terminal output, and file contents are never notification data.
Delivery uses the Wails native notification runtime and does not add a helper
process or sidecar. The macOS bundle uses the stable identifier
`dev.johannes.shhh` so authorization belongs to a consistent application
identity.

On macOS, `make build` produces `build/bin/shh-h.app`. The embedded Go
executable is `build/bin/shh-h.app/Contents/MacOS/shhh`.

## Structure

- `cmd/shhh`: executable entry point and Wails project configuration.
- `internal/app`: composition root and desktop lifecycle configuration.
- `internal/domain`: pure profile, transfer, SSH, tunnel, snippet, workspace,
  remote-path favorite, settings, and notification models.
- `internal/port`: terminal, SSH connection, and remote filesystem contracts.
- `internal/adapter`: PTY, SSH, known-host, SFTP, session-log, native
  notification, profile exchange, and configuration adapters.
- `internal/usecase`: profile, session, transfer, SSH, tunnel, snippet,
  remote-path favorite, notification, and workspace orchestration.
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
