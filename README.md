# shh-h

`shh-h` is a modular Go desktop terminal and SSH toolbox in the style of
MobaXterm. It packages the Go backend and React frontend as a self-contained
native desktop application.

## What Works

- A Wails v2 native host with an embedded React and strict TypeScript frontend.
- Real interactive local shells through a pseudoterminal on macOS and Linux and
  native ConPTY on supported Windows systems.
- xterm.js rendering, input, resize, search, titles, bells, and persistent tabs.
- Searchable terminal-tab switching, pointer and command reordering, Ctrl+Tab
  cycling, and accessible roving tab focus without remounting live terminals,
  backed by a rendered 50-tab switching, hidden-output, and disposal stress test.
- Bounded two-pane terminal splits with side-by-side or stacked layouts,
  pointer and keyboard resizing, explicit pane focus, and no controller
  reparenting or implicit session launch.
- State-aware terminal actions for retry-in-place, reconnecting while preserving
  old output, duplicating running tabs, clearing scrollback, and resetting the
  local terminal emulator.
- Searchable local and SSH profiles with create, edit, duplicate, delete,
  grouping, tags, favorites, validated local-shell environment overrides,
  versioned atomic persistence, private JSON export, and strict atomic import
  from shh-h JSON or concrete OpenSSH hosts.
- One-off SSH quick connect with transient validated targets and the same strict
  host-key verification and credential handling as saved profiles.
- Interactive SSH terminals with strict known-host verification, explicit
  first-use trust, agent, key, password, and keyboard-interactive authentication.
- An SFTP browser with streamed upload/download, configurable bounded
  concurrency, explicit collision policies, progress, cancellation, atomic
  partial files, explicit restart-safe resume/discard actions, upload integrity
  verification, directory operations, permissions, and profile-scoped
  remote-path favorites.
- Saved local, remote, and dynamic SOCKS5 SSH tunnels with explicit lifecycle,
  actual bound addresses, retry policy, auto-start policy, and loopback defaults.
- Saved command snippets with strict variables, backend-rendered previews, and
  guarded execution across explicitly selected live terminals.
- Opt-in per-session output logging with optional line timestamps, private file
  permissions, bounded rotation, visible state, and terminal-owned cleanup.
- Versioned terminal settings for font, spacing, cursor, scrollback, and bell
  behavior, with live application to open terminals and durable reset support.
- Versioned SSH connection settings for bounded connect/handshake deadlines and
  application-level keepalives with a configurable unanswered-probe threshold.
- Shared reference-counted SSH connection groups let terminals, SFTP
  workspaces, and tunnels reuse one authenticated transport without closing
  each other's active channels.
- Versioned transfer settings for concurrency, destination collisions, and
  failed partial-file retention, with live application to queued work.
- Opt-in native notifications for long completed transfers and failed terminal
  sessions, with explicit OS permission, category controls, and a test action.
- A searchable command palette with grouped actions, disabled-state feedback,
  keyboard navigation, and shell-safe application shortcuts.
- Terminal viewport copy and exact selection export through native clipboard
  and Save dialogs, with bounded private atomic text files.
- OSC 8 and detected web links with strict HTTP/HTTPS canonicalization, visible
  confirmation, and system-browser handoff instead of WebView navigation.
- Saved workspace layouts with private atomic persistence, ordered profile-tab
  and split-arrangement snapshots, disconnected restoration, and explicit
  per-tab reconnection.
- Explicit session ownership, activation, ordered input/output, bounded output
  flow control, frontend leases, and deterministic shutdown of terminals,
  transfers, SFTP clients, and tunnels.
- Single-instance handling and confirmation before closing live resources.
- A single macOS application bundle with no web server, Node runtime, daemon,
  or sidecar process required at runtime.

The current packaged application and full terminal-performance proof target is
macOS arm64. Linux has a native PTY and headless WebKitGTK focus, clipboard, and
inactive-render-suspension CI gate on Ubuntu 24.04 amd64. The Windows ConPTY
backend has a native Windows CI gate, while WebView2 focus, AltGr, IME,
clipboard, and broader interaction validation remain open. Signed/notarized
release automation, advanced reconnect/proxy preferences, and remaining
cross-platform UX are also future milestones; see the implementation plan for
the release gates.

## Prerequisites

- Go 1.26.5 or newer.
- Node.js 24.18 or newer and npm 11 for frontend development.
- Windows 10 version 1809 or newer for ConPTY sessions, plus a supported system
  WebView2 runtime for the desktop UI.
- Linux builds require GTK 3 and WebKitGTK 2.41 or newer through the
  `webkit2gtk-4.1` ABI. Ubuntu 24.04 amd64 is the current native CI baseline.

Node and npm are build-time dependencies only. Make installs the pinned Wails
v2.13.0 CLI into the ignored local `bin/` directory when it is first needed.

## Commands

```sh
make run       # foreground Wails development mode; Ctrl+C stops it
make test      # Go and frontend unit/integration tests
make lint      # ESLint and go vet
make build     # native package with embedded build identity
make check-bindings # regenerate, normalize, and verify Wails bridge contracts
./scripts/run-terminal-benchmark-macos.sh # packaged WKWebView performance gate
./scripts/run-terminal-soak-macos.sh # 15-minute, 8-session native soak gate
```

`make run` is intentionally a foreground developer command and exits with its
terminal. Packaged applications launch through Finder or the target operating
system's desktop application launcher and do not require a parent terminal.

## Build Identity

`make build` embeds version, source revision, UTC build date, and dirty-tree
state into the Go executable. Settings displays those fields together with the
Go version and target platform, which makes development packages and future
release diagnostics identifiable without a helper process or network call.

The default development version is `0.1.0-dev`; commit, build date, and dirty
state are read from the current checkout. Release automation can provide
repeatable values explicitly:

```sh
make build VERSION=1.0.0 COMMIT=0123456789ab \
  BUILD_DATE=2026-07-17T18:00:00Z DIRTY=false
```

Direct Go builds fall back to Go's embedded VCS revision and modified flag.
Their build date remains `unknown` unless supplied through the linker.

## Continuous Integration

GitHub Actions runs normal and race-enabled Go tests, `go vet`, frontend lint,
tests, and production compilation on Ubuntu. A macOS job installs the pinned
Wails CLI from a clean checkout, performs a production-mode native compile,
regenerates the Go-to-TypeScript bridge, normalizes generator-only whitespace,
and fails if any binding differs from the committed contract. The Linux job
checks the documented WebKitGTK floor, runs real PTY resize/process-tree cleanup
tests, compiles the product host, and launches a production-mode xterm/Wails
smoke under Xvfb for native focus, clipboard, and hidden-render pause/resume
checks. The Windows job runs the real ConPTY adapter tests before compiling the
Wails desktop host.

The security job runs the Go team's call-graph vulnerability scanner and fails
for reachable advisories. It also runs `npm audit --audit-level=high`; moderate
and lower reports remain visible but do not fail CI. GitHub Actions are pinned
to immutable full commit SHAs with their release tags in comments. Dependabot
checks Go modules, frontend packages, and Actions weekly.

Run the binding gate locally with `make check-bindings`. The current audit
policy and module-only advisory review are recorded in `docs/SECURITY.md`.

## Terminal Text Actions

`Copy visible terminal` copies only the active viewport, joins xterm soft wraps,
and ignores right-side cell padding. `Export terminal selection` is enabled only
while the active terminal has selected text and writes that exact selection to
a user-chosen `.txt` or `.log` file. Exports are capped at 16 MiB, use private
permissions, and are atomically replaced; cancelling the native dialog writes
nothing. Both actions are available from the terminal toolbar and command
palette.

## Terminal Tab Actions

The terminal-actions button and command palette expose only actions valid for
the active tab state. `Retry terminal` releases any remaining exited or failed
backend runtime before replacing that tab. `Reconnect in new tab` preserves the
old tab and its output, while `Duplicate terminal tab` opens another connection
only from a running tab. Saved profiles and transient quick-connect tabs both
re-enter the normal host-key and credential flow; passwords and passphrases are
never retained for reuse.

`Clear scrollback` uses xterm's bounded local buffer operation. `Reset terminal`
performs a local RIS reset of the emulator. Neither action writes bytes to the
shell or remote host, and both become inert after controller disposal.

## Terminal Split Layout

`Split terminal right` and `Split terminal down` show the next existing tab in
a second pane; they never create a shell or network connection. At most two
panes are visible. Choosing a hidden tab replaces the focused pane, while
`Close terminal split` returns to the focused session without closing either
tab. The divider supports pointer dragging, arrow-key steps, Home and End bounds,
and double-click balancing, with a fixed 20–80% size range.

Every live terminal retains its original stage host and controller while split
geometry changes around it. Saved workspace layouts persist the two tab indexes,
axis, ratio, and focused tab. Restoring a layout still creates disconnected tabs
only; each session must be connected explicitly.

## Local Profile Environment

Local profiles expose environment overrides as explicit variable/value rows in
the profile editor. Names must use the portable shell form
`[A-Za-z_][A-Za-z0-9_]*`, contain at most 128 characters, and are unique without
regard to letter case. Profiles are limited to 128 entries. Empty values are
preserved. Overrides replace matching values from the inherited process
environment when the PTY starts.

`TERM`, `COLORTERM`, and `SHHH_SESSION_ID` are owned by the terminal runtime and
cannot be overridden. Environment values are stored in private profile
configuration and included in profile exports. They are not a secret store; do
not use them for passwords, access tokens, or other credentials.

## Terminal Resource Limits

The Go session manager admits at most 64 open terminal sessions across local,
saved SSH, quick-connect, and benchmark workflows. Connecting sessions reserve
capacity before a PTY or network transport is allocated, so concurrent opens
cannot oversubscribe the limit. A failed connection returns its reservation;
an admitted session retains its slot until the terminal is explicitly closed,
including after the remote process exits. The UI presents a typed,
actionable error when the limit is reached.

xterm scrollback is bounded separately through terminal settings. The standard
renderer remains the supported baseline and WebGL is disabled; any future
WebGL path must add its own visible-pane context limit before it is enabled.

## SSH Connection Policy

Connection settings default to a 15-second limit for TCP connection and SSH
handshake setup. The supported range is 3 through 120 seconds. The same saved
deadline applies to host-key probes, saved and quick SSH terminals, SFTP, and
SSH tunnels.

SSH keepalives are enabled by default every 30 seconds with an unanswered-probe
threshold of three. They use application-level `keepalive@openssh.com` requests
and bounded request concurrency, so a silent network failure eventually closes
the SSH client instead of leaving an apparently connected runtime indefinitely.
Keepalives can be disabled or configured from 5 through 300 seconds with a
threshold from one through ten. Saved changes are captured by newly opened SSH
connections; existing connections retain the policy with which they were
opened.

## SSH Connection Sharing

Terminals, SFTP workspaces, and tunnels opened for the same effective SSH
profile share one authenticated client while their work overlaps. Each feature
owns a separate reference-counted lease, so closing a terminal closes only its
channel when an SFTP workspace or tunnel still needs the connection. Closing
the final lease immediately closes the SSH client and its waiter and keepalive
work; there is no idle background connection cache.

Connection keys contain profile and SSH identity fields, but never credentials
or terminal dimensions. Credentials are used only by the caller that performs
the first dial and are cleared by the owning use case. Concurrent first opens
wait for that one dial, and a dead connection is removed before the next open
redials. Starting the application or restoring a workspace still opens no
network connection.

## Remote Path Favorites

Inside an open SFTP workspace, use the star beside the path field to add or
remove the current directory. The adjacent Favorites menu navigates to saved
paths for that profile. Favorites are canonical absolute remote paths stored in
a separate private atomic file; loading them never opens a connection, and
choosing one uses only the SFTP session that is already visible.

## Transfer Policies

Transfer settings default to two active uploads or downloads, asking before a
destination is replaced, and removing failed partial files. The concurrency
limit can be changed from one to eight while work is queued. Raising it wakes
queued transfers immediately; lowering it waits for active work to finish and
does not cancel an in-progress transfer.

Collision behavior can ask, overwrite, skip, or keep both. Ask uses a native
dialog with Keep Both as the safe default. Skips remain visible in the transfer
list, and Keep Both reserves numbered names such as `report (1).csv` across
both existing files and concurrently queued transfers. Downloads choose a
destination folder and retain the remote basename.

Uploads and downloads always write a hidden `.shhh-part-<transfer-id>` file and
publish the destination only after success. By default failed and cancelled
partials are removed; remote cleanup is necessarily best effort if the SFTP
transport itself has already been lost. Enabling Keep partial files records a
private, versioned resume entry before bytes are copied. Interrupted entries
survive an application restart and expose explicit Resume and Discard actions
after an SFTP workspace for the same saved profile is opened. Nothing reconnects
or resumes automatically at startup.

Before resuming, downloads require the same remote size and modification time
and a regular local partial with a valid size. Uploads additionally require the
same local source SHA-256 digest, compare the remote partial prefix with the
local source, and verify the completed remote partial before publication.
Destination collisions are checked again and every resume owns an exclusive
in-flight reservation. Successful transfers and explicit discards remove their
metadata; records whose source or partial changed remain visible but can only be
discarded.

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
- `cmd/terminalbench`: development-only packaged-macOS benchmark and Linux
  native-smoke host and process sampler; it is not included in release artifacts.
- `internal/app`: composition root and desktop lifecycle configuration.
- `internal/buildinfo`: linker-provided and Go VCS build identity.
- `internal/domain`: pure profile, transfer, SSH, tunnel, snippet, workspace,
  remote-path favorite, settings, and notification models.
- `internal/port`: terminal, SSH connection, and remote filesystem contracts.
- `internal/adapter`: PTY, SSH, known-host, SFTP, session-log, native
  notification, profile exchange, and configuration adapters.
- `internal/usecase`: profile, session, transfer, SSH, tunnel, snippet,
  remote-path favorite, notification, and workspace orchestration.
- `internal/terminalbenchmark`: guarded content-free metrics, fixed same-binary
  PTY fixture, report validation, and provisional performance budgets.
- `internal/bridge`: narrow typed Wails command and event boundary.
- `frontend`: React terminal, profile, file, transfer, tunnel, snippet, layout,
  and settings workspaces.
- `docs/IMPLEMENTATION_PLAN.md`: milestones, acceptance criteria, and release
  scope.
- `docs/SECURITY.md`: automated audit policy and reviewed advisory notes.
- `docs/WINDOWS.md`: ConPTY requirements, lifecycle contract, native tests, and
  remaining Windows interaction gates.
- `docs/LINUX.md`: WebKitGTK baseline, native PTY/WebView smoke gate, local
  reproduction, and remaining Linux coverage.
- `docs/TERMINAL_STRESS.md`: real-PTY lifecycle, process, descriptor, goroutine,
  and frontend-listener stress evidence.
- `docs/TERMINAL_BENCHMARK.md`: reproducible packaged WKWebView terminal
  throughput, latency, queue, memory, and close-response evidence.
- `docs/TERMINAL_SOAK.md`: reproducible packaged WKWebView long-duration,
  multi-session, memory-growth, responsiveness, and cleanup evidence.
- `docs/REMOTE_PROJECTS_PLAN.md`: proposed self-hosted remote code editor,
  provisioning, project lifecycle, and browser-authentication design.
- `docs/TELEPORT_INTEGRATION_PLAN.md`: proposed Teleport cluster, browser
  authentication, resource discovery, terminal, and compliance design.
- `docs/adr/0001-desktop-frontend-stack.md`: accepted frontend decision and
  tradeoffs.
- `docs/adr/0002-terminal-performance-budgets.md`: accepted native terminal
  queue, latency, completion, close, scrollback, and process-memory budgets.
- `docs/adr/0003-native-terminal-soak-budgets.md`: accepted multi-session soak,
  steady-state memory-growth, responsiveness, and cleanup budgets.

The dependency direction and runtime ownership rules are documented in
`docs/ARCHITECTURE.md`.
