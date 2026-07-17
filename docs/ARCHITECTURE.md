# Architecture

## Goals

`shh-h` is a modular, cross-platform MobaXterm-style desktop tool for terminal
sessions, SSH, file transfer, and tunnels. It should be testable, secure, and
distributed as one self-contained application package per operating system.

The core rule is that process, network, profile, credential, transfer, and
tunnel behavior never depends on the desktop framework. Wails hosts the window
and bridge, React presents application workflows, xterm.js owns terminal
emulation, and Go owns every operating-system or network resource.

## Accepted Frontend Stack

- Stable Wails v2 for the native desktop host and Go/TypeScript bridge.
- React with strict TypeScript for application UI and state.
- Stable xterm.js for terminal parsing, input, scrollback, accessibility, and
  rendering.
- An exact Node 24 LTS and npm toolchain for Vite builds. Node and npm are
  build-time tools only; production frontend assets are embedded in the Go
  executable.
- Wails v3 is not eligible while it remains alpha.

The decision and alternatives are recorded in
`docs/adr/0001-desktop-frontend-stack.md`.

## Dependency Direction

```text
React and xterm.js
        |
        v
Wails bridge and DTOs
        |
        v
Go use cases -> domain models <- ports <- adapters
        ^                                  |
        +----------------------------------+

cmd/shhh -> internal/app wires the complete application
```

- Domain models use only the Go standard library.
- Use cases depend on domain models and narrow ports.
- PTY, SSH, SFTP, session-log, config, settings, workspace-layout,
  remote-path-favorite, profile-exchange, and secret adapters implement those
  ports.
- The Wails bridge maps task-oriented commands and typed events to use cases.
- React never receives backend objects and never stores terminal output in
  component state.
- One non-React terminal controller owns each xterm.js instance.
- React StrictMode remains enabled in development. Component mount effects may
  attach terminal hosts and listeners but never create backend resources.

## Package Boundaries

### `cmd/shhh`

Small executable entry point. It delegates startup to `internal/app` and stays
free of product logic.

### `internal/app`

Composition root and lifecycle coordinator. It embeds the production frontend,
constructs adapters and services, starts Wails, and closes all resources during
shutdown.

### `internal/domain`, `internal/usecase`, and `internal/port`

Pure product models, validation, state machines, orchestration, and narrow
interfaces. These packages have no Wails, React, PTY, SSH, or storage details.
Interfaces are introduced at their consumer only for a real platform boundary,
alternate implementation, or focused test replacement. The architecture does
not require generic repositories, event buses, or empty package scaffolding.

Saved connection profiles enter through the profile service and its atomic
repository. Quick SSH targets are normalized into transient profile values by
the SSH connection use case; caller IDs and persistence metadata are discarded,
and no quick-connect operation can write the profile repository.

### `internal/adapter`

Concrete local PTY, Windows ConPTY, SSH, SFTP, config, known-hosts, and native
secret-store implementations.

### `internal/bridge`

The only privileged frontend boundary. It exposes typed commands for profiles,
sessions, transfers, tunnels, snippets, settings, remote-path favorites, and
workspace layouts and emits typed lifecycle events. Terminal
output uses ordered byte-safe chunks, per-session generations, bounded queues,
cumulative byte-credit acknowledgement, fair scheduling, and backpressure.
Lifecycle and control traffic does not wait behind bulk terminal output.
Saved and quick SSH commands share the same trust, authentication, terminal,
lease, and credential-clearing services. Quick commands accept only the narrow
connection fields required for a one-off terminal.

### `frontend`

React application, feature views, shared components, styles, and terminal
controllers. xterm.js receives terminal bytes directly from the bridge;
terminal data does not pass through React rendering. One persistent host and
controller exists per live tab; a restored disconnected tab has neither a
backend session nor a terminal controller. `onData` and `onBinary` feed one
ordered input queue; resize is coalesced without losing the final dimensions.
Palette and shortcut actions call the same React workflow callbacks as visible
controls; they do not add a second bridge path or create backend resources on
their own.

Terminal text actions are read-only controller operations. Copy Visible reads
the active xterm viewport and sends it directly to the Wails native clipboard;
Export Selection reads xterm's exact selection and passes it to a bounded
private atomic-file adapter after the user chooses a path in the native Save
dialog. Neither payload enters React component state, session logs, layout
storage, or profile storage. Remote-controlled terminal titles are sanitized
before they become suggested export filenames.

## Remote Path Favorite Ownership

A remote-path favorite is configuration, not a live SFTP resource. Its private
versioned store contains only a generated ID, saved-profile ID, canonical
absolute POSIX path, and creation timestamp. It never contains a hostname,
username, credential, trust decision, SFTP session ID, directory listing, or
transfer state. The bridge accepts new favorites only for an existing SSH
profile and rejects duplicate profile/path pairs.

Loading favorites performs no network operation. The frontend shows only those
owned by the currently open saved profile; choosing one issues an ordinary
navigation request through that already-open SFTP session. Favorites whose
profiles were removed remain inert and hidden rather than being reassigned.

## Workspace Layout Ownership

A saved layout is declarative UI state, not a session checkpoint. Its versioned
private store contains a layout name, ordered profile IDs, display-only title
and endpoint snapshots, and the selected tab index. It never contains terminal
output, runtime IDs, credentials, trust decisions, environment secrets, or
serialized xterm state. Deleted profile references remain readable so the UI
can show an unavailable disconnected tab instead of corrupting the layout.

Workspace-layout persistence commands never call the PTY, SSH, SFTP, tunnel,
lease, or session managers. Frontend restoration first confirms and closes any
live terminal tabs, then creates frontend-only disconnected tabs; it never
creates a runtime. Connect is a separate user action that resolves the current
profile and enters the existing host-key and credential workflow. Quick-connect
targets are transient and are omitted when capturing a layout.

## Session Ownership

The Go session manager owns every live process and connection. Each runtime has
a context, transport, state machine, output pump, waiter, frontend lease,
session generation, and idempotent close. The frontend refers to it by session
ID and generation. Closing a live tab closes the corresponding runtime;
closing a disconnected tab removes only its frontend metadata. Closing the
application coordinates shutdown of all sessions, transfers, and tunnels.

A DOM instance attaches through a frontend lease. The lease is renewed only
while backend resources are live. Frontend reload, replacement, or loss closes
lease-owned resources after a bounded grace period in 1.0; implicit process
reattachment is not supported. Old-generation commands and events are harmless.

Wails `OnBeforeClose` starts visible confirmation and coordinated shutdown.
`OnShutdown`, which runs after frontend destruction, is only the final
idempotent cleanup path. `SingleInstanceLock` ensures one process owns writable
configuration and runtimes; secondary launches focus the primary window.

Starting the application starts no shell and opens no network connection. No
tray daemon or hidden helper survives application exit.

## Single-Application Strategy

- Wails embeds compiled HTML, CSS, JavaScript, icons, and migrations.
- No Node runtime, local HTTP server, helper daemon, or sidecar executable is
  required in production.
- Only embedded origins can call bound Go methods. Production disables remote
  scripts and navigation, developer tools, and the default WebView context menu.
- macOS uses its system WKWebView and ships as an `.app` bundle.
- Windows ships as a GUI `.exe` and requires a supported system WebView2
  runtime.
- Linux uses a documented WebKitGTK runtime and ships as an executable and
  optional package/AppImage.
- User config, secrets, known hosts, logs, and transferred files remain
  external user-owned data.

## Immediate Milestones

1. M0: migrate the scaffold from Fyne to stable Wails v2 and React while
   preserving backend tests.
2. M1: prove a real macOS PTY through the Wails bridge into xterm.js, including
   cumulative flow control, frontend-loss handling, binary input, ordering,
   fairness, measured performance, resize, and cleanup.
3. M2: productionize local terminals on macOS/Linux and add Windows ConPTY,
   with native focus, AltGr, IME, clipboard, and lifecycle gates.
4. M3 onward: session workspace, profiles, secure SSH, SFTP, tunnels, product
   UX, packaging, and hardening.

The complete gates and release scope are in `docs/IMPLEMENTATION_PLAN.md`.
