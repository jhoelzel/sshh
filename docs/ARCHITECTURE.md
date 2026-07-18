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
  remote-path-favorite, profile-exchange, native-notification, and secret
  adapters implement those ports.
- The Wails bridge maps task-oriented commands and typed events to use cases.
- React never receives backend objects and never stores terminal output in
  component state.
- One non-React terminal controller owns each xterm.js instance.
- React StrictMode remains enabled in development. Component mount effects may
  attach terminal hosts and listeners but never create backend resources.
- The main bootstrap and independent notification-status query each share a
  nonce-scoped promise while in flight, so StrictMode's effect replay cannot
  duplicate either command group. Settled data is not cached: a genuine later
  root mount refreshes its data and lease.
- Every bridge subscription owns its Wails disposer, and terminal-controller
  disposal is idempotent. Unmounting therefore removes only that mount's
  listeners and controllers.

## Error Contract

Use cases and bridge commands classify rejected operations through
`internal/apperror`. Wails formats every backend rejection into a stable JSON
envelope, and the frontend bridge client converts it into `BackendError` before
feature code observes it. Wrapped causes remain available to Go diagnostics but
are not serialized to the WebView. The code list, retry rules, and authoring
guidance are documented in `docs/ERROR_HANDLING.md`.

## Application Startup Ownership

`internal/app` creates only an inert bridge and Wails options before entering
the native event loop. It does not open a config repository, run a migration,
create an SSH pool, or start a background monitor at that point.

The lifecycle order is intentional:

1. `OnStartup` records the Wails context so an early second-instance request can
   be queued or can activate the primary window. It performs no composition.
2. Wails completes its native `SingleInstanceLock` decision before the initial
   page can become DOM-ready on macOS, Linux, and Windows. This distinction is
   important because Wails v2 invokes `OnStartup` before its Linux lock setup.
3. The first `OnDomReady` composes adapters and use cases exactly once, injects
   them through a host-only desktop controller, and starts the frontend lease
   monitor. Later DOM-ready notifications cannot create another runtime.
4. React's lazy bootstrap waits on the sole public `AwaitReady` command before
   importing the product application. Backend startup failures release the same
   wait with a typed, cause-redacted error and leave the startup boundary visible.
5. `OnShutdown` and the fallback after `wails.Run` share one idempotent close
   path. Desktop-owned sessions, transfers, tunnels, notifications, monitors,
   and the SSH client pool are closed in ownership order.

The desktop controller is not included in Wails bindings. JavaScript cannot
configure, start, or shut down backend services. A secondary process exits in
Wails' native instance setup before DOM-ready and therefore cannot open a
writable store. Its callback focuses the existing window; callbacks that arrive
before context preparation are retained and delivered once preparation finishes.

The pinned Wails lifecycle ordering is part of the platform contract. A Wails
upgrade must re-check all three native frontend implementations and rerun the
two-process smoke gate before this boundary is considered preserved.

## Package Boundaries

### `cmd/shhh`

Small executable entry point. It delegates startup to `internal/app` and stays
free of product logic.

### `internal/app`

Composition root and lifecycle coordinator. It embeds the production frontend,
starts Wails with an inert bridge, constructs adapters and services only after
the native instance decision, and closes all resources during shutdown.

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

Local-profile environment overrides are validated in the profile domain before
persistence or import. The React editor preserves values exactly while
requiring portable, case-insensitively unique names. The session manager emits
overrides in deterministic order and adds its owned session identifier; the PTY
adapter then merges them over the inherited process environment while retaining
runtime ownership of terminal capability variables. Environment values are
never emitted as diagnostics or lifecycle events.

### `internal/adapter`

Concrete local PTY, Windows ConPTY, SSH, SFTP, config, known-hosts, and native
secret-store implementations.

The Windows terminal adapter uses typed `x/sys/windows` APIs plus one contained
raw `UpdateProcThreadAttribute` call for the opaque HPCON value. It starts the
root process suspended, assigns a private kill-on-close job before resuming, and
explicitly nulls inherited standard handles so a console-attached parent cannot
bypass ConPTY. Synchronous input and output remain independently serviceable.
Closing the transport can interrupt blocked pipe I/O. Natural teardown keeps
output draining while `ClosePseudoConsole` runs on a separate goroutine, with a
bounded forced-close fallback for undrained output on older supported Windows
releases.

### `internal/bridge`

The only privileged frontend boundary. It exposes typed commands for profiles,
sessions, transfers, tunnels, snippets, settings, notifications, remote-path
favorites, and workspace layouts and emits typed lifecycle events. Terminal
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
Output frames are checked for the current lease and generation, safe sequence
and offset values, the 64 KiB event cap, canonical payload length, and decoded
byte count before xterm receives them. A malformed frame remains retryable at
the same sequence; an xterm parser failure halts that controller's output so a
later cumulative acknowledgement cannot skip rejected bytes.
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

Terminal links follow one path for xterm OSC 8 hyperlinks and URLs found by the
official web-links addon. The controller accepts only absolute HTTP and HTTPS
URLs, rejects credentials, controls, backslashes, malformed hosts, and values
over 2,048 characters, and emits the canonical URL to React. React displays that
exact value in a confirmation dialog, validates it again, and only then calls
Wails' system-browser handoff. Terminal output cannot navigate the application
WebView directly, and non-web schemes remain inert.

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

## Transfer Policy Ownership

Transfer preferences are durable application settings, while each transfer
captures the active policy when it starts. The bridge owns native path and
conflict dialogs. The transfer manager owns collision resolution, destination
reservations, queue admission, partial-file lifecycle, progress, cancellation,
and final publication. React displays typed transfer states and never performs
filesystem collision checks itself.

The worker limiter is wakeable rather than a fixed-capacity channel. Increasing
the configured limit admits queued work immediately. Decreasing it prevents new
admission until active work drops below the new limit and never cancels work
already running. The limit remains bounded from one through eight.

Ask, overwrite, skip, and keep-both policies apply to both local download
destinations and remote upload destinations. Ask returns control to the bridge
for a native Replace, Keep Both, or Cancel decision. Skip creates a visible
terminal transfer state. Keep Both reserves its numbered destination before
queueing, so concurrent requests cannot select the same candidate before either
file exists. Filesystem state is checked again before non-overwriting final
renames to fail safely if an external process wins a race.

Every transfer writes a generated hidden `.shhh-part-<transfer-id>` path and
renames it only after a successful close and, for local downloads, sync. Failed
partials are removed by default. When retention is enabled, the manager writes a
versioned private resume record before copying and updates it after interruption.
The record contains a generated ID, saved-profile ID, canonical source,
destination and partial paths, source size and modification time, captured
collision policy, progress, and an upload source SHA-256 digest. It contains no
credential, SSH client, frontend lease, SFTP session, or file-content data.

Resume records survive process restart but are never executed automatically.
They become visible only after the user opens an SFTP workspace for the matching
saved profile, and React can request only explicit Resume or Discard commands.
The manager serializes each record, reacquires an exclusive destination
reservation, and rechecks non-overwrite collisions before final publication.
Downloads require an unchanged remote size and modification time and use
`Lstat` plus file-identity checks so a local partial cannot be replaced by a
symlink between validation, append, and rename. Uploads require the original
local SHA-256 digest, compare the local and remote partial prefixes, and verify
the complete remote partial digest before rename. Success and discard remove
the record; failed validation preserves it as unavailable for explicit cleanup.
Remote cleanup remains best effort after transport loss.

## Connection Policy Ownership

Connection preferences are durable application settings and are exposed to SSH
adapters through a narrow read-only settings source. Reading or changing them
does not open a socket. Each host-key probe or authenticated SSH dial snapshots
the latest validated policy; already-open terminals, SFTP clients, and tunnels
retain their original policy until they close.

The configured connection timeout bounds both the TCP dial and SSH handshake
through one derived context. The same deadline path is used for saved profiles,
quick connections, host-key probes, SFTP, and tunnels. This avoids divergent
network behavior between trust and authenticated workflows.

When enabled, each authenticated SSH connection group owns one context-bound
keepalive coordinator. It sends `keepalive@openssh.com` global requests, treats
either a positive or negative protocol reply as proof of transport liveness,
bounds outstanding requests by the configured failure threshold, and closes
the client after that threshold remains unanswered. The group context ends only
after transport failure, final-lease release, or application shutdown; closing
one feature context cannot stop keepalives needed by another active lease. The
connection settings store is schema-versioned; older documents migrate to
conservative defaults before validation.

## SSH Connection Ownership

`internal/adapter/sshclient.Pool` owns authenticated `ssh.Client` instances.
Terminals, SFTP filesystems, and tunnel attempts acquire independent leases;
their adapters own only feature-specific channels, subsystems, listeners, and
relay connections. Closing a terminal therefore closes its session before
releasing its lease, while closing an SFTP workspace closes its SFTP client
before releasing the same kind of lease.

Groups are keyed by saved or deterministic quick-profile ID plus normalized
host, port, username, authentication mode, and identity file. Credentials and
terminal dimensions are intentionally excluded: secrets are supplied only to
the first dial and are never retained by the pool. Editing connection fields
while an old group is active creates a distinct group. Concurrent first
acquisitions wait for one bounded dial instead of racing duplicate handshakes.

Each group has exactly one connection waiter and at most one keepalive
coordinator. A lease waiter also observes that lease's own close signal, so a
tunnel can stop without leaving a goroutine blocked on a connection retained by
another feature. Remote closure evicts the group before a later acquisition
redials. Final-lease release removes the group, cancels maintenance, closes the
client, and waits for the connection waiter within a fixed bound. Application
shutdown cancels in-flight dials and defensively closes all remaining groups.
There is deliberately no idle reuse timeout yet, so zero references means zero
background SSH connections.

## Notification Ownership

System notifications are an optional presentation side effect, not a runtime
resource and not a second application event bus. Session and transfer managers
continue to publish their existing typed state events. The desktop event sink
passes only failed terminal states and completed transfers to the notification
use case, which applies the current durable preferences and then delegates to a
narrow Wails adapter. Delivery runs outside the event publisher so an operating
system notification failure cannot stall terminal or transfer lifecycle work.

The adapter is initialized with the Wails application context during desktop
startup and cleaned up during final shutdown. Initialization does not request
authorization. Permission is requested only by the dedicated bridge command
invoked from the visible Settings action. Application enablement and operating
system authorization remain separate states; delivery requires both, and a
denied or revoked permission does not mutate application preferences.

Notification payloads are bounded and sanitized before crossing the adapter.
Transfer messages contain only direction, duration, and the source or
destination basename. Session messages contain the bounded display title and
failure summary. They never contain credentials, terminal output, file
contents, full filesystem paths, or profile configuration. Explicit session
close follows the normal closed state and is not treated as a failed-session
notification.

## Session Ownership

The Go session manager owns every live process and connection. Each runtime has
a context, transport, state machine, output pump, waiter, frontend lease,
session generation, and idempotent close. The frontend refers to it by session
ID and generation. Closing a live tab closes the corresponding runtime;
closing a disconnected tab removes only its frontend metadata. Closing the
application coordinates shutdown of all sessions, transfers, and tunnels.

The manager also owns one bounded output dispatcher while any session is
registered. Output pumps submit at most one in-flight chunk each; the dispatcher
serializes bridge delivery and rotates session IDs after every chunk. Terminal
input, resize, close, and lifecycle state bypass that queue. Closing the final
session signals dispatcher shutdown without waiting for an in-flight GUI event
callback, while lease and generation checks make any late output harmless.

A DOM instance attaches through a frontend lease. The lease is renewed only
while backend resources are live. Frontend reload, replacement, or loss closes
lease-owned resources after a bounded grace period in 1.0; implicit process
reattachment is not supported. Old-generation commands and events are harmless.

Frontend attachment attempts are serialized. A repeated nonce renews the same
lease, while a new nonce invalidates the old lease and does not return until the
terminal, SFTP, and tunnel managers have concurrently closed every old-lease
resource. The replacement frontend therefore cannot receive a usable lease
while an old-lease shell is still running. The real-PTY flood, process-tree,
resource-baseline, and frontend-listener evidence is recorded in
`docs/TERMINAL_STRESS.md`.

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
- Windows 10 version 1809 or newer ships as a GUI `.exe`, uses native ConPTY,
  and requires a supported system WebView2 runtime.
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
