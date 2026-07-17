# Implementation Plan

## 1. Implementation Status

The macOS core slices through M8, the first M9 productivity slice, and the
terminal portion of M10 are implemented. The repository contains a Wails v2
host, embedded React and strict TypeScript frontend, xterm.js terminal
controllers, a real Unix PTY adapter, strict SSH and known-host adapters, SFTP,
saved tunnel state, and lease-owned runtime managers with bounded bridge flow
control.

Implemented and verified:

- No shell starts with the application; local PTYs start only from an explicit
  profile action and always have a visible tab.
- Session IDs, generations, and renewable frontend leases reject stale bridge
  traffic and close resources after frontend loss.
- PTY output is held until the xterm controller activates, sent as ordered
  chunks, and bounded by a cumulative acknowledgement window.
- Terminal input is ordered and idempotent, resize is validated and debounced,
  and close escalates from hangup to termination to kill within bounded time.
- The macOS arm64 application launches as a self-signed `.app` with all
  frontend assets embedded in its Go executable.
- Profiles use a strict versioned schema, atomic private-file replacement,
  external-change detection, and legacy migration backup.
- Profile exchange uses a versioned credential-field-free JSON document,
  private atomic exports, and one-save imports. Concrete OpenSSH hosts import
  with first-value precedence and visible diagnostics; unsafe proxy directives
  are skipped rather than converted into unintended direct connections.
- One-off SSH quick connect creates only a validated transient profile. It
  reuses strict host-key and authentication workflows and can open a terminal
  without writing profile configuration.
- SSH supports explicit first-use trust, changed-key rejection, agent, key,
  password, and keyboard-interactive authentication without persisting secrets.
- SFTP operations stream through bounded workers and partial files; the file
  workspace exposes navigation, upload, download, rename, delete, mkdir, chmod,
  progress, and cancellation.
- Local, remote, and SOCKS5 tunnels have saved independent models, loopback
  defaults, guarded public binds, lifecycle events, bounded relays, cancellation,
  retry, and explicitly enabled auto-start.
- Command snippets use a strict private store, validated variables, an exact
  backend-rendered preview, explicit live targets, and confirmation for
  multi-terminal execution.
- Session logging is opt-in, output-only, privately stored, timestamp-capable,
  bounded by rotation, visible while active, and owned by terminal shutdown.
- Terminal defaults for font, size, spacing, cursor, scrollback, and bell use a
  validated versioned store, apply live to open controllers, and reset
  durably.
- The command palette searches grouped connection, profile, navigation, and
  active-terminal actions. Arrow-key operation and non-conflicting
  `Cmd/Ctrl+Shift` shortcuts work without intercepting ordinary shell input.
- Go race tests cover managers and adapters. Real loopback integration tests
  cover PTY, SSH terminal resize/exit, SFTP operations, and bidirectional local,
  remote, and SOCKS forwarding. TypeScript, ESLint, Vitest, vet, and production
  builds pass.

Still required for the complete cross-platform and 1.0 gates:

- ConPTY support and Windows validation.
- Multi-session fairness and the documented throughput, memory, and long-run
  stress measurements.
- Accessibility-driven native interaction automation or a dedicated Wails E2E
  harness. macOS launch and frontend attachment are verified today; runtime
  behavior is also exercised below the WebView boundary.
- Shared reference-counted SSH connection groups, resumable transfer metadata,
  connection and transfer settings, saved workspace
  layouts, notifications, and the remaining productivity
  actions.
- Signed/notarized macOS releases, Linux packaging validation, Windows WebView2
  and ConPTY implementation, native CI, accessibility review, and long-run
  soak/performance evidence.

## 2. Product Definition

### 2.1 Core 1.0

Version 1.0 will be a production-usable MobaXterm-style core client with:

- A native desktop workspace with searchable profiles and multiple session
  tabs.
- Interactive local shells using a real pseudoterminal.
- Interactive SSH shells with password, keyboard-interactive, private-key, and
  SSH-agent authentication.
- Strict SSH host-key verification and an explicit first-connection trust flow.
- A usable xterm-compatible terminal with colors, Unicode, scrollback,
  selection, clipboard, keyboard shortcuts, resize, alternate screen, and mouse
  reporting.
- UTF-8 terminal sessions in 1.0. Legacy character-set conversion is a
  post-1.0 feature and is never guessed silently from terminal output.
- Profile create, edit, duplicate, delete, import, group, tag, and quick-connect
  workflows.
- Local and remote file browsing over SFTP, including a visible transfer queue.
- Saved local, remote, and dynamic SSH tunnels with explicit lifecycle state.
- Session logs, snippets, and guarded multi-session input.
- Settings for shell, terminal, appearance, connection behavior, and data
  locations.
- Native macOS, Linux, and Windows builds from one Go codebase.
- One self-contained application package per target. Frontend assets are
  embedded in the Go executable and no helper daemon, local web server, Node
  runtime, or sidecar executable is required. Platform WebViews and system
  frameworks are treated as operating-system prerequisites.

Core 1.0 means a complete and reliable daily-use SSH toolbox. It does not mean
pixel-for-pixel or protocol-for-protocol parity with every feature accumulated
by MobaXterm.

### 2.2 Post-1.0 Parity Track

After the core is stable, parity expands in this order:

1. WSL distributions and richer Windows shell discovery.
2. Serial, Telnet, and raw TCP sessions, with prominent security warnings for
   plaintext protocols.
3. X11 forwarding to an existing local X server.
4. An embedded X server, treated as a separate major subsystem with its own
   protocol, rendering, clipboard, font, and security test plan.
5. RDP and VNC sessions. Native system-client integration comes before any
   embedded implementation.
6. Compile-time extension registration for additional protocols without
   runtime plugins or sidecar binaries.

Mosh and other tools that require an external client or server binary are not
part of the strict single-application promise unless they are reimplemented or
linked into the application.

### 2.3 Optional Remote Projects Track

A post-1.0 Remote Projects track may provision a user-scoped, self-hosted Code
OSS editor on an SSH host, save remote directories as local projects, and open
them through an ephemeral SSH-backed loopback gateway. It also includes a
strict external-browser bridge for compatible remote CLI authentication flows.
It does not install a desktop editor on the client or use a hosted tunnel
control plane.

This work changes the current no-local-web-server runtime promise while a
remote editor is open and introduces third-party provider, license, supply
chain, extension, and OAuth boundaries. It therefore requires its own ADRs and
release gates and is not folded into M9 workspace layouts. The proposed design
and acceptance criteria are in `docs/REMOTE_PROJECTS_PLAN.md`.

## 3. Non-Negotiable Behavior

### 3.1 Session Visibility and Background Work

- Starting the application starts no shell and opens no network connection.
- A local process starts only after the user chooses New Local Terminal or
  connects a local profile.
- An SSH connection starts only after the user explicitly connects a profile.
- Every running terminal has a visible tab and state indicator.
- An unfocused tab may continue running, as normal terminal tabs do, but it is
  never invisible background work.
- A live Go runtime belongs to one frontend lease and one session generation.
  Stale commands and events from an older frontend or session generation are
  rejected.
- A frontend reload, crash, or replacement cannot leave an unowned process or
  connection running. While live resources exist, a lightweight frontend lease
  is renewed; losing it starts bounded shutdown unless a deliberately designed
  reattachment protocol is added later.
- Closing a running tab prompts when needed, terminates its process or SSH
  channel, waits for completion, and removes it from the runtime registry.
- Closing the main window presents one consolidated confirmation if sessions,
  transfers, or tunnels are active. Confirming shutdown stops all of them.
- The application has no tray-only mode and leaves no daemon or child shell
  behind after normal exit.
- Only one application instance owns the runtime and writable configuration.
  A secondary launch focuses the existing window and passes it any supported
  launch request.
- Tunnels and transfers may continue while another tab is selected, but they
  remain visible in a global Activity view.

### 3.2 Failure Behavior

- Failed connections become visible failed sessions with a useful reason and a
  Retry action.
- Network loss never pretends that a session is connected.
- Frontend loss is a lifecycle failure, not an invitation to buffer terminal
  output indefinitely. Backend and bridge queues remain bounded while the
  frontend lease expires, then all lease-owned resources close.
- Development hot reload follows the same ownership rule. Version 1.0 closes
  active runtimes on frontend replacement instead of attempting an unsafe
  implicit reattachment.
- Terminal sessions are not silently resumed because an arbitrary shell cannot
  be resumed safely. Reconnect opens a new channel; tmux or screen remains the
  user's explicit persistence mechanism.
- Partial file downloads use a temporary name and are never presented as
  complete files.
- A host-key mismatch is a hard failure, not a warning that can be casually
  bypassed.

## 4. Architecture

### 4.1 Dependency Direction

```text
frontend/                    React UI and one xterm.js controller per terminal
   |
   | generated bindings, commands, and typed events
   v
internal/bridge              narrow Wails API, DTO mapping, output flow control
   |
   v
internal/usecase             session, profile, transfer, tunnel services
   |
   v
internal/domain              pure models, validation, state machines
   ^
   |
internal/port                transport, storage, secrets, clock interfaces
   ^
   |
internal/adapter             PTY, SSH, SFTP, config, keychain implementations

cmd/shhh -> internal/app     Wails host, embedding, composition, shutdown
```

Dependency rules:

- `domain` imports only the standard library.
- `usecase` depends on `domain` and narrow interfaces from `port`.
- `adapter` implements ports and may use external libraries.
- `bridge` exposes task-oriented commands and typed DTOs, not storage or
  transport objects.
- `frontend` depends only on generated bridge contracts and frontend modules.
- Only `app` constructs adapters, use cases, the bridge, and the Wails host.
- Transport implementations do not know about tabs, dialogs, or widgets.
- React state contains session metadata, never terminal output or xterm buffers.
- Persistence stores data models, not live runtime objects.

### 4.2 Planned Package Layout

```text
cmd/shhh/
internal/app/
internal/domain/profile/
internal/domain/session/
internal/domain/transfer/
internal/domain/tunnel/
internal/usecase/profile/
internal/usecase/session/
internal/usecase/transfer/
internal/usecase/tunnel/
internal/port/terminal.go
internal/port/profile_store.go
internal/port/secret_store.go
internal/port/host_keys.go
internal/adapter/localpty/
internal/adapter/sshclient/
internal/adapter/sftpclient/
internal/adapter/configstore/
internal/adapter/secretstore/
internal/bridge/
internal/platform/
frontend/src/app/
frontend/src/component/
frontend/src/feature/profile/
frontend/src/feature/session/
frontend/src/feature/terminal/
frontend/src/feature/files/
frontend/src/feature/tunnels/
frontend/src/lib/bridge/
frontend/src/styles/
frontend/test/
assets/
build/
docs/adr/
```

Packages will be introduced only when their milestone begins. The layout is a
boundary map, not permission to create empty directories.

Architecture remains deliberately shallow:

- Build each milestone as a vertical slice through the smallest required
  packages.
- Define an interface at the consuming package only when there is a real
  platform boundary, a second implementation, or a focused test replacement.
- Do not introduce generic repositories, service bases, event buses, command
  buses, or dependency-injection frameworks without demonstrated complexity.
- Prefer explicit constructors and typed calls. A package must own a coherent
  capability rather than merely mirror a directory diagram.

### 4.3 Transport Contract

Local PTY and SSH shell implementations will satisfy the same contract:

```go
type TerminalTransport interface {
	io.Reader
	io.Writer
	Resize(ctx context.Context, columns, rows uint16) error
	Signal(ctx context.Context, signal Signal) error
	Wait(ctx context.Context) (ExitStatus, error)
	Close() error
}
```

The contract intentionally deals in terminal bytes and dimensions. PTY,
ConPTY, and SSH details remain in adapters.

### 4.4 Runtime Ownership

Each live session is a pointer-owned runtime object containing:

- An immutable profile snapshot.
- A session ID generated independently from the profile ID.
- A monotonically increasing generation that changes if an ID is ever reused
  or a replacement runtime is opened.
- The frontend lease ID that owns the runtime.
- A context and cancellation function.
- Exactly one terminal transport.
- A typed state machine.
- One output pump, one waiter, a sequence counter, and bounded bridge flow
  control.
- An idempotent `Close` guarded by `sync.Once`.
- A `WaitGroup` that makes shutdown observable and testable.

The session manager owns the Go runtime registry. A frontend tab stores the
runtime ID and generation, serializable metadata, and a non-React
`TerminalController` that owns one xterm.js instance. Runtime state is never
represented by copied Go structs, and terminal bytes never pass through React
component state. Creating a runtime is an explicit user command; React mounting
or remounting a component never opens a process or connection.

Valid state transitions are:

```text
created -> connecting -> connected -> closing -> closed
                    \-> failed --------^          ^
created ----------------> canceled ---------------|
```

State changes are typed events. Transport output is batched into ordered,
byte-safe chunks for the bridge. The frontend terminal controller feeds chunks
directly into xterm.js and sends a cumulative acknowledgement after xterm has
accepted them. A per-session byte-credit window bounds unacknowledged backend
output and pending frontend writes. High and low watermarks reduce bridge call
frequency; output is never acknowledged once per byte or forced to use one RPC
per chunk. Flow control slows the transport reader when the frontend cannot
keep up. Output is never silently dropped and queues never grow without a cap.

Control and lifecycle traffic has priority over bulk output. Fair scheduling
between sessions prevents one noisy terminal from starving input, resize,
close, lifecycle events, or another session. Every event is checked against its
frontend lease and session generation before delivery.

### 4.5 Desktop Bridge Contract

The Wails bridge exposes commands equivalent to:

```text
AttachFrontend(instanceNonce) -> FrontendLeaseDTO
RenewFrontendLease(frontendLeaseID)
OpenTerminal(frontendLeaseID, profileID, columns, rows) -> SessionDTO
WriteTerminal(sessionID, generation, inputSequence, payloadBase64)
ResizeTerminal(sessionID, generation, columns, rows)
SignalTerminal(sessionID, generation, signal)
CloseTerminal(sessionID, generation)
AcknowledgeTerminalOutput(sessionID, generation, throughSequence, bytesConsumed)
```

The frontend lease is renewed only while live backend resources exist, so an
idle application performs no heartbeat or polling. A new DOM instance gets a
new lease. Replacing or expiring a lease closes its runtimes in 1.0.

Output events carry frontend lease ID, session ID, session generation,
monotonically increasing sequence, byte count, a byte-safe payload, and an
explicit final marker where appropriate. Acknowledgements are cumulative and
idempotent. M1 chooses the framing and batch size using measurements; base64 is
the correctness baseline and may be replaced only by a proven binary path.
Lifecycle events are separate from terminal data so a slow terminal cannot
hide disconnect or exit state. Late events after close are harmless and are
discarded by generation.

Input from xterm `onData` and `onBinary` enters one serialized per-session
queue. The controller preserves callback order even though Wails commands are
asynchronous, converts binary reports without Unicode reinterpretation, chunks
large paste operations, and never has more than a bounded number of writes in
flight. Resize is coalesced and debounced, but the final dimensions are always
sent.

The bridge contract is generated or checked in CI to prevent Go and TypeScript
models from drifting. No remote origin receives access to bound Go methods.

### 4.6 SSH Connection Ownership

An SSH connection group owns one authenticated `ssh.Client`. Terminal channels,
SFTP clients, and tunnels take reference-counted leases from that group. Closing
one terminal channel does not kill an active transfer or tunnel. Closing the
last lease closes the underlying client unless the profile has an explicitly
enabled connection-reuse timeout.

## 5. Technical Decisions and Gates

### 5.1 Desktop and Frontend Stack

- Use the latest reviewed stable Wails v2 release, pinned exactly in `go.mod`.
- Pin the Wails CLI to the same release as the Go module so generated bindings
  and packaging behavior cannot drift.
- Pin an exact stable Go patch release supported by that Wails version through
  `go.mod`, the Go toolchain directive, and CI. Toolchain upgrades are reviewed
  changes, not implicit consequences of a developer machine update.
- Do not adopt Wails v3 while it is alpha. Reconsider it only after a stable
  release and a measured migration benefit.
- Use React with strict TypeScript for application state and workflows.
- Keep React StrictMode enabled in development. Effects may attach and detach UI
  resources, but they never create a backend runtime. All subscriptions and
  controller disposal paths are idempotent under StrictMode remounting.
- Use an exact Node 24 LTS toolchain selected in M0, recorded in `.nvmrc` or
  `.tool-versions`, enforced by `package.json` `engines`, and matched in CI.
  Pin npm through `packageManager` and use Vite with `npm ci` and a committed
  lockfile for reproducible frontend builds. Review the pinned LTS patch through
  dependency updates rather than silently following the local machine.
- Keep terminal I/O outside React state and rendering.
- Use stable Lucide React icons and a small token-based CSS layer rather than a
  large component framework.
- Node and npm are build-time tools only. Production runs the embedded frontend
  in the operating system WebView.

### 5.2 Terminal Emulator

Use a pinned stable release of `@xterm/xterm` for terminal parsing, buffering,
input encoding, accessibility, and rendering. Initially allow only maintained
stable addons for fit and search. The WebGL addon is optional and must fall back
cleanly when unavailable. Beta builds, proposed APIs, and experimental addons
are excluded from the 1.0 dependency baseline.

One controller and one persistent terminal host element exist for each open
tab. Hiding a tab does not destroy or recreate its controller. Fit and renderer
work run only for visible terminal panes. The standard renderer is the 1.0
baseline; if WebGL is enabled later, it is limited to visible panes with a hard
context cap and deterministic disposal.

Go owns terminal transports and lifecycle but does not duplicate xterm.js state
or parse ANSI for display. We will not hand-roll an ANSI parser as application
glue. Terminal sequences are untrusted input and must never trigger local
command execution, unrestricted navigation, or clipboard writes.

### 5.3 Bridge Performance and Backpressure Gate

M1 must prove the complete path from a real macOS PTY through Go and Wails into
xterm.js, plus the return path for input and resize. The gate measures ordering,
invalid UTF-8 handling, batching latency, sustained throughput, bounded memory,
paste behavior, inactive tabs, and shutdown during output.

React must not re-render for terminal chunks. xterm.js `write` completion drives
the consumed-byte count for cumulative acknowledgement and flow control. It
means xterm has parsed the write, not that pixels are guaranteed to have been
painted. Acknowledgements are emitted at a low-watermark transition or at most
once per display frame, not as one bridge command per output chunk.

The M1 implementation begins with these provisional hard limits:

- At most 64 KiB of raw terminal bytes per output event and at most one display
  frame of batching latency.
- At most 1 MiB of unacknowledged backend output and 1 MiB of pending frontend
  writes per session, excluding the separately capped xterm scrollback buffer.
- Input and lifecycle traffic is serviced within 100 ms even while another
  session is producing sustained output.
- Packaged-macOS local input echo is at most 50 ms p95 while idle and 150 ms p95
  during the 10 MiB output test; resize reaches the PTY within 150 ms p95 after
  the final resize event.
- The 10 MiB stream completes within 10 seconds without duplication, reordering,
  loss, an unbounded queue, or an unresponsive close action.

M1 records the hardware, workload, and measurements. A provisional number may
change only through a short benchmark decision record before M2; later
milestones treat the accepted numbers as regression budgets.

### 5.4 Lifecycle and Frontend Ownership

- `OnStartup` constructs backend services but does not call window APIs that
  require a ready frontend.
- `OnDomReady` marks the host ready for the frontend attachment handshake.
- `OnBeforeClose` owns close interception. With active resources it prevents
  the first close, requests one consolidated frontend decision, and permits a
  programmatic close only after coordinated backend shutdown completes.
- `OnShutdown` runs after the frontend is gone. It is a final idempotent cleanup
  and assertion path, never the first signal on which frontend acknowledgement
  depends.
- Wails `SingleInstanceLock` is enabled from M0. A secondary instance sends a
  bounded launch request to the primary instance, focuses it, and exits without
  opening config stores or runtimes.
- A frontend lease is renewed only while sessions, transfers, or tunnels are
  live. Normal bridge traffic also counts as renewal. M1 selects a bounded
  health-check interval and expiry grace using minimized-window, UI-stall, and
  system sleep/wake tests; a healthy minimized or resumed app must never lose a
  session. Lease expiry, frontend replacement, window close, and explicit quit
  all converge on the same cancellation and wait path.
- Shutdown has a five-second total budget after user confirmation. It requests
  graceful closure first, escalates platform processes when necessary, and
  reports any forced termination before the window exits when the frontend is
  still available.

### 5.5 Local Pseudoterminals

- macOS and Linux use `github.com/creack/pty` behind a Unix adapter.
- Windows uses ConPTY behind a Windows adapter and build tags. The adapter will
  use the native pseudoconsole API through a small, reviewed Go wrapper.
- The default Unix shell comes from the user's account or `SHELL`, with a safe
  platform fallback. Windows profiles discover PowerShell, PowerShell Core,
  Command Prompt, and WSL explicitly.
- Child processes receive `TERM=xterm-256color` and a correct initial window
  size.
- Unix processes run in their own process group so tab and application shutdown
  can terminate the complete child tree.

### 5.6 SSH and SFTP

- SSH uses `golang.org/x/crypto/ssh`.
- Known-host parsing and verification use
  `golang.org/x/crypto/ssh/knownhosts`.
- Agent communication uses `golang.org/x/crypto/ssh/agent`.
- SFTP uses `github.com/pkg/sftp` through an internal filesystem interface.
- Every network dial has a context, connect timeout, keepalive policy, and
  deterministic close path.

### 5.7 Secrets

Profiles contain only credential references. Passwords and key passphrases are
never written to profile JSON. A `SecretStore` port uses macOS Keychain, Windows
Credential Manager, and Linux Secret Service where available. Session-only
credentials remain in memory and are discarded on close. Logs, errors, and
diagnostic exports pass through a redaction layer.

WebView prompts have a documented limitation: submitting a secret necessarily
creates JavaScript and bridge serialization data that cannot be reliably
zeroed. Agent and OS-keychain authentication therefore appear before manual
secret entry. Password and passphrase controls are isolated, uncontrolled
inputs; their values never enter React state, shared stores, browser storage,
history, telemetry, or diagnostics and are cleared immediately after the
bridge call settles. Production developer tools are disabled. M5's threat-model
gate decides whether this bounded exposure is acceptable or whether a native
secret prompt is required before SSH ships.

### 5.8 Single-Application Distribution

- Wails embeds the production frontend, icons, default themes, migrations, and
  other static assets in the Go executable.
- Production adds no remote `BindingsAllowedOrigins`: only embedded app content
  may call bindings. It rejects remote bridge callers, disables the default
  WebView context menu and developer tools, and never loads remote scripts.
- The runtime does not unpack or execute helper programs.
- Production requires no Node runtime and starts no local HTTP server.
- Windows ships as one GUI-subsystem `.exe` with no console window.
- Windows 10/11 must have a supported WebView2 runtime. Packaging detects it and
  provides the official bootstrap path when absent; a fixed WebView runtime is
  not shipped as an application sidecar.
- Linux uses a documented minimum WebKitGTK runtime and offers the raw
  executable plus an optional AppImage/package.
- macOS necessarily ships as an `.app` bundle because that is the platform's
  application format. It uses the system WKWebView and contains one app-owned
  executable with no helper service.
- User profiles, secrets, known hosts, logs, and transfer data remain external
  user-owned runtime data.
- Native build and smoke-test jobs begin in M0 and expand when each platform
  adapter lands. Cross-compilation alone is never treated as runtime proof.

### 5.9 Platform Promotion Order

Platform support advances through native gates rather than one simultaneous
claim:

1. macOS arm64 is the M1 development and packaged-runtime proof platform.
2. Windows amd64 must pass native ConPTY, WebView2, keyboard focus, tab
   traversal, AltGr, IME, clipboard, resize, and shutdown tests during M2.
3. Linux amd64 must pass native PTY, WebKitGTK, keyboard, clipboard, packaging,
   and shutdown tests during M2.

M3 cannot claim a cross-platform workspace until all three native local-terminal
gates pass. macOS amd64 joins the release matrix in M11. Later features may be
implemented on macOS first, but a milestone is not complete until its stated
native platform matrix passes.

## 6. Milestone Plan

Each milestone ends in a runnable application. Placeholder tabs and fake
connected states are not considered progress once their milestone begins.

### M0: Foundation and Engineering Gates

Deliverables:

- Record the accepted Wails v2, React/TypeScript, and xterm.js decision in an
  ADR, including alternatives and the M1 validation gate.
- Preserve backend behavior with tests, scaffold the Wails host and frontend,
  then remove Fyne and its transitive dependencies.
- Pin an exact stable Go toolchain, Node 24 LTS release, npm, Wails CLI, Go
  dependencies, and frontend dependencies. Configure strict TypeScript, React
  StrictMode, linting,
  formatting, frontend unit tests, `package-lock.json`, and reproducible
  `npm ci` builds.
- Generate or validate Go-to-TypeScript bridge contracts in CI.
- Record process lifecycle, SSH trust, secrets, WebView security, and
  single-application distribution decisions.
- Replace the current generic backend package shape with domain, use-case, port,
  and adapter boundaries as code is touched.
- Introduce typed errors and structured state transitions.
- Establish `go test`, race tests, formatting, linting, vulnerability scanning,
  frontend dependency checks, and native build checks.
- Add build metadata: semantic version, commit, build date, and dirty state.
- Add a root application context and coordinated shutdown service.
- Configure `OnStartup`, `OnDomReady`, `OnBeforeClose`, and `OnShutdown` with
  idempotent ownership, and enable `SingleInstanceLock` before writable config
  migration begins.
- Lock bridge origins to embedded content, disable production developer tools
  and the default context menu, block remote navigation, and verify that the
  production build starts no HTTP listener.
- Remove fake active-session counts and the example.com default profile.
- Keep only actions that perform real work; unfinished features remain absent
  rather than showing success-like placeholders.

Tests and exit gate:

- Current config behavior is covered before migration.
- The application starts and exits without spawning a child process.
- Repeated startup and shutdown leave no goroutines or files open.
- A second application launch focuses the first instance and exits without
  opening a second writable config store.
- React StrictMode's development remount does not duplicate bridge listeners,
  controllers, commands, or backend resources.
- The Wails application builds and launches as an arm64 `.app` on the current
  Mac using the system WKWebView.
- Native macOS, Linux, and Windows CI jobs build the desktop shell, using
  temporary stubs only for platform adapters not reached by the UI.
- Production assets load from the embedded filesystem with networking disabled.

### M1: Wails and xterm.js End-to-End Terminal Proof

Deliverables:

- Pin a stable xterm.js release and the stable fit and search addons.
- Build a `TerminalController` that owns xterm outside React state and disposes
  every listener and addon deterministically. React only attaches its persistent
  host element and never opens a runtime from a mount effect.
- Implement the typed Wails commands and lifecycle/output events for a single
  diagnostic terminal.
- Implement the frontend attachment lease, active-resource-only renewal, lease
  expiry, and session generation checks. Frontend replacement closes the
  diagnostic runtime within the shutdown budget.
- Implement a minimal Darwin PTY adapter sufficient for a real local shell
  spike, with explicit process cleanup.
- Preserve raw PTY bytes through output events containing lease ID, session ID,
  generation, sequence, raw byte count, byte-safe payload, and final marker.
- Implement a cumulative byte-credit window with the provisional batch and
  queue limits, high/low watermarks, bounded frontend/backend queues, and
  transport backpressure.
- Connect both xterm `onData` and `onBinary` through one ordered input queue.
  Add monotonic input sequence numbers, bounded in-flight writes, large-paste
  chunking, and debounced/coalesced resize without routing bytes through React
  state.
- Prioritize input, resize, close, and lifecycle traffic and fairly schedule
  output across diagnostic sessions used by the stress harness.
- Implement fit, scrollback, selection, copy, bracketed paste, search, focus,
  terminal title, and bell indication using stable public APIs.
- Use xterm's standard renderer first. Evaluate WebGL only as an optional
  acceleration path with automatic fallback.
- Allow only sanitized HTTP and HTTPS links to open through the operating
  system. Disable remote navigation and OSC 52 clipboard writes by default.
- Add a production Content Security Policy and disable production developer
  tools and unneeded WebView capabilities.

Tests and exit gate:

- The real shell supports typing, Ctrl+C, resize, paste, selection, copy,
  scrollback, and clean exit on the current Apple Silicon Mac.
- `vim`, `less`, `top`, `tmux`, color output, Unicode, emoji, combining marks,
  and IME input render and behave correctly.
- Sequence tests prove no duplicated, reordered, or silently dropped chunks.
- Duplicate cumulative acknowledgements are harmless; stale commands and late
  events from an old generation or frontend lease are rejected or discarded.
- Interleaved `onData`, `onBinary`, and paste input reaches the PTY in callback
  order, including binary mouse reports, while resize delivers the final size.
- Invalid UTF-8 and malformed terminal streams do not crash Go, Wails, or the
  WebView.
- A 10 MiB output burst remains interactive and memory stays within explicit
  queue and scrollback caps and passes the provisional latency, fairness, and
  completion budgets in section 5.3.
- The same flood in one diagnostic session cannot starve input, lifecycle
  events, or a second low-volume diagnostic session.
- Repeated React StrictMode attach/detach cycles and a frontend reload create no
  duplicate shell and leave no old-lease shell running.
- Minimizing the window, a deliberate main-thread stall, and system sleep/wake
  do not expire a healthy frontend lease. Simulated frontend loss does expire it
  and reap the shell within the measured bounded grace period.
- Closing the tab or window during heavy output reaps the shell and returns
  goroutine, descriptor, and bridge-listener counts to baseline.
- `OnBeforeClose` performs the visible decision and coordinated shutdown;
  `OnShutdown` remains safe when invoked after frontend destruction.
- A packaged arm64 macOS `.app` passes the same smoke workflow.

If this end-to-end gate fails, implementation stops before broader UI work and
the host/bridge design is revisited using the measurements. It does not fall
back to a hand-built terminal emulator by default.

### M2: Real Local Terminal Vertical Slice

Deliverables:

- Implement the common `TerminalTransport` contract.
- Productionize Unix PTY startup, input, output, resize, signal, wait, and
  cleanup from the M1 Darwin spike, then add Linux coverage.
- Implement Windows ConPTY startup, input, output, resize, wait, and cleanup.
- Add local shell discovery and profile options for executable, arguments,
  working directory, environment overrides, and login-shell behavior.
- Wire explicit New Local Terminal and Connect actions to a live runtime.
- Replace the scaffold terminal label with reusable xterm-backed session tabs.
- Update title, exit status, running duration, and current directory when safely
  available through terminal metadata.
- Make Ctrl+C, Ctrl+D, Ctrl+Z, resize, paste, and full-screen TUI applications
  behave correctly.
- Implement close escalation: request graceful exit, wait briefly, terminate the
  process group, then force kill only if required.
- Always reap the child process.

Tests and exit gate:

- Integration tests execute `printf`, read input, report the PTY size, resize,
  return an exit code, and terminate a child process tree.
- Closing a tab leaves no process, file descriptor, goroutine, or runtime entry.
- Closing the application with a running shell follows the confirmation and
  cleanup contract.
- Opening 100 short-lived terminals in a test loop produces no lifecycle leak.
- No shell starts on application launch or mere profile selection.
- Native Windows tests cover WebView focus restoration, forward and reverse tab
  traversal, AltGr, IME composition, clipboard shortcuts, ConPTY resize, and
  close during output; browser-only tests do not satisfy this gate.
- Native Linux tests cover WebKitGTK focus, clipboard, PTY resize, process-group
  cleanup, and the documented minimum runtime version.

This is the first production-quality milestone that turns the proof into a real
cross-platform program.

### M3: Session Workspace and Tab Lifecycle

Deliverables:

- Add closeable, reorderable session tabs with protocol and state icons.
- Add keyboard navigation, new-tab commands, tab search, and focus restoration.
- Support multiple simultaneous terminal runtimes without shared mutable xterm
  controllers or React component state.
- Keep one persistent terminal host/controller per open tab. Hidden tabs retain
  terminal state without repaint work, are not unmounted by ordinary tab
  selection, and refit when made visible.
- Cap open-session resources and scrollback. If optional WebGL is enabled, use
  it only for visible panes under a tested graphics-context cap.
- Add clear connecting, connected, disconnected, failed, and exited states.
- Add Retry, Reconnect in New Tab, Duplicate Tab, Clear Scrollback, Reset
  Terminal, and Close actions.
- Add split-terminal layout only after tabs are stable.
- Add a global Activity view for sessions, transfers, and tunnels.
- Add a single shutdown coordinator and consolidated close confirmation.
- Persist window geometry, sidebar width, selected theme, and non-sensitive UI
  preferences. Never attempt to resurrect dead processes on restart.

Tests and exit gate:

- State-machine tests reject invalid transitions.
- Concurrent tab open, output, resize, and close passes the race detector.
- React component tests and Playwright flows cover focus, shortcuts, tab close,
  split layout, and shutdown decisions.
- Native Wails smoke tests cover window close interception and lifecycle hooks.
- An inactive running tab is still visible and consumes no redraw work unless
  its model changes.
- Stress tests cover at least 50 open tabs, sustained output in several hidden
  tabs, repeated active-tab changes, and deterministic controller disposal.

### M4: Profiles, Configuration, and Migration

Deliverables:

- Replace the bare profile array with a versioned config document.
- Add deterministic migrations and backup-before-migration behavior.
- Preserve atomic writes, add file sync where supported, and enforce private
  permissions.
- Add profile CRUD, duplicate, folders/groups, tags, favorites, sorting, and
  filtering.
- Add quick connect without requiring a saved profile.
- Add protocol-specific forms that show only relevant fields.
- Add SSH config import for hosts, user, port, and identity files with OpenSSH
  first-value precedence. Proxy and other unsupported directives are reported;
  connection-critical options that cannot be represented skip the affected
  host rather than silently changing its route.
- Add import/export that deliberately excludes runtime IDs, timestamps, and
  dedicated credential fields.
- Add terminal defaults per profile with global fallbacks.
- Detect external file changes or conflicting edits before overwriting
  configuration. Process-level single-instance ownership is already enforced
  from M0.

Tests and exit gate:

- Every schema version has forward migration fixtures.
- Corrupt and truncated config files produce a recoverable error with backup
  options.
- Duplicate IDs and names do not overwrite profiles.
- Profile validation tests cover IPv4, IPv6, hostnames, ports, shell paths,
  proxy chains, and invalid combinations.

### M5: Credentials and SSH Trust

Deliverables:

- Implement the cross-platform secret-store port.
- Add session-only, remember-in-keychain, and SSH-agent credential choices.
- Prefer agent and keychain choices before manual password/passphrase entry.
- Parse unencrypted and encrypted OpenSSH private keys.
- Prompt for key passphrases and keyboard-interactive challenges without
  exposing responses in React state, shared frontend stores, browser storage,
  logs, diagnostics, or immutable profile data. Clear isolated uncontrolled
  inputs immediately after submission.
- Read the user's OpenSSH known-hosts file and maintain an application-specific
  known-hosts file.
- Show algorithm and SHA-256 fingerprint on first contact.
- Require explicit trust for a new host and distinguish permanent from
  session-only trust.
- Treat changed and revoked keys as hard failures with explanatory details.
- Add safe diagnostic logging with host and profile IDs but no passwords,
  passphrases, private keys, terminal contents, or file contents.

Tests and exit gate:

- Mock secret stores cover unavailable, locked, denied, and deleted secrets.
- Known, unknown, changed, revoked, hashed, IPv6, and non-default-port host keys
  are covered.
- Authentication cancellation stops the dial and leaves no partial session.
- A repository-wide test asserts that serialized profiles never contain secret
  material.
- Frontend tests prove secret controls never dispatch values to application
  state or persistence. A threat-model review explicitly accepts the WebView
  string-lifetime limitation or requires a native prompt before M6.

### M6: Interactive SSH

Deliverables:

- Dial with `net.Dialer` and context deadlines, then perform the SSH handshake.
- Support password, keyboard-interactive, private key, agent, and ordered
  fallback authentication.
- Request an `xterm-256color` PTY with the terminal's actual dimensions.
- Forward terminal input and output through the common runtime.
- Send SSH window-change requests after UI resize.
- Support startup directory and startup command without unsafe string
  concatenation.
- Add configurable keepalives and server-alive failure thresholds.
- Add proxy jump through one or more SSH profiles with loop detection.
- Add optional agent forwarding with a prominent per-profile opt-in.
- Make disconnect reasons and remote exit status visible.
- Introduce SSH connection groups and leases for terminal, SFTP, and tunnel
  reuse.

Tests and exit gate:

- Integration fixtures cover each auth method, host-key trust, PTY resize,
  Unicode, exit status, connection timeout, abrupt disconnect, and cancellation.
- Proxy-jump and connection-lease lifecycle tests close every hop in reverse
  order.
- No code path uses `ssh.InsecureIgnoreHostKey`.
- Closing the final lease closes the socket and waiter goroutines.

### M7: SFTP Browser and Transfer Manager

Deliverables:

- Implement remote filesystem operations through a narrow filesystem port.
- Add local and remote panes with path navigation, sorting, hidden-file toggle,
  refresh, bookmarks, and keyboard operation.
- Add create directory, rename, delete with confirmation, chmod, upload,
  download, and open-with-system actions.
- Add a transfer manager with bounded concurrency, queueing, progress, speed,
  ETA, cancellation, retry, and per-transfer errors.
- Download to a temporary partial file and atomically rename on success.
- Support resumable upload/download where server capabilities and metadata make
  it safe.
- Define symlink behavior explicitly and prevent accidental recursive cycles.
- Keep transfers alive when a terminal tab closes only if another SSH
  connection lease and visible Activity item remain.

Tests and exit gate:

- In-process or container-backed SFTP tests cover listing, Unicode names,
  rename, permissions, symlinks, interruption, resume, cancellation, and
  checksum comparison.
- Local destination collision policies are explicit: ask, overwrite, skip, or
  rename.
- Transfer queue memory is bounded and large files are streamed, never loaded
  fully into memory.
- Disconnecting during transfer produces a resumable failed item, not a false
  success.

### M8: SSH Tunnels

Deliverables:

- Add saved tunnel models independent of terminal profiles.
- Implement local forwarding, remote forwarding, and dynamic SOCKS5 forwarding.
- Add bind address, requested port, destination, profile, startup policy, and
  reconnect policy.
- Default local bind addresses to loopback and warn before binding all
  interfaces.
- Show starting, active, retrying, failed, and stopped states with actual bound
  addresses.
- Handle every accepted connection in a cancellable child context.
- Use SSH connection leases so stopping a tunnel does not disrupt unrelated
  channels.
- Add explicit Start, Stop, Restart, Edit, and View Error actions.

Tests and exit gate:

- Local, remote, and SOCKS forwarding pass bidirectional integration tests.
- Port collision, denied remote forwarding, DNS failure, network loss, and
  cancellation are covered.
- Stopping a tunnel closes its listener and every relayed connection.
- No tunnel auto-starts unless the user explicitly enabled that tunnel.

### M9: Productivity Features

Deliverables:

- Add reusable command snippets with folders, tags, variables, and a preview
  before execution.
- Add guarded multi-execution mode with a persistent visual warning and an
  explicit target-session list.
- Add optional session logging with start/stop controls, timestamp policy,
  rotation, and secure file permissions.
- Add terminal search, copy-all-visible, and export-selection actions.
- Add saved workspace layouts that restore profile tabs as disconnected tabs;
  reconnection remains explicit.
- Add profile and remote-path favorites.
- Add command palette and consistent keyboard shortcuts.
- Add notifications for long transfer completion and unexpected disconnect,
  respecting OS and application settings.

Tests and exit gate:

- Snippet variables are escaped or sent exactly as previewed.
- Multi-execution cannot target password/passphrase dialogs or hidden sessions.
- Logging is off by default and never captures credential prompts supplied by
  the application.
- Restoring a workspace starts no process or network connection automatically.

### M10: Settings, Accessibility, and UX Completion

Deliverables:

- Add terminal font, size, line spacing, cursor, palette, scrollback, copy,
  paste, bell, and hyperlink policies.
- Add connection timeout, keepalive, reconnect, proxy, known-hosts, and agent
  settings.
- Add transfer concurrency, collision, partial-file, and notification settings.
- Add reset-to-default and per-profile override indicators.
- Add screen-reader labels, keyboard traversal, visible focus, contrast checks,
  and reduced-motion behavior.
- Replace transient status-label messaging with actionable dialogs, inline
  validation, and an activity/error history.
- Ensure compact desktop layouts and smaller windows remain usable without text
  overlap.

Tests and exit gate:

- Settings round-trip, migration, reset, and override precedence are tested.
- All primary workflows are keyboard reachable.
- Automated layout tests cover minimum supported desktop sizes and high-DPI
  scaling.

### M11: Cross-Platform Packaging and Release Pipeline

Deliverables:

- Add embedded application icon, metadata, licenses, and default assets.
- Promote the native CI jobs introduced in M0/M2 into the complete release
  matrix for macOS arm64/amd64, Linux amd64, and Windows amd64.
- Build release-mode binaries with version metadata and stripped debug symbols.
- Package the Wails host as macOS `.app`, Windows GUI `.exe`, and Linux
  executable/package forms.
- Add code signing and notarization hooks, with unsigned local-development
  builds remaining easy.
- Generate checksums, SBOM, dependency license report, and release notes.
- Verify clean-machine startup and expected config, cache, log, and keychain
  paths on every OS.
- Verify WebView prerequisites and failure messages on supported Windows and
  Linux versions.
- Document that `make run` is a foreground developer command while packaged apps
  launch through the desktop environment.

Tests and exit gate:

- Each release artifact launches on a clean supported OS image.
- Windows opens no console window.
- macOS launches from Finder and passes signing/notarization verification when
  credentials are configured.
- The application contains no unexpected network listeners or unpacked helper
  executable.
- Embedded frontend assets and migrations work after relocation and without
  network access.

### M12: 1.0 Hardening

Deliverables:

- Run full unit, integration, UI, race, fuzz, and cross-platform suites.
- Add goroutine and file-descriptor leak checks around every long-lived service.
- Run `govulncheck`, dependency review, static analysis, and manual threat-model
  review.
- Profile startup, idle CPU, terminal flood, scrollback memory, large directory
  listing, large transfer, and many-session behavior.
- Add structured local diagnostics and a user-controlled diagnostic export with
  redaction.
- Complete user documentation for profiles, trust prompts, credentials,
  transfers, tunnels, settings, data locations, and recovery.
- Remove every placeholder, fake-success state, debug action, and dead code
  path.

Release gate:

- Idle with no sessions performs no polling and has no sustained background
  work.
- All resources close deterministically on tab close and application exit.
- Core workflows pass on macOS, Linux, and Windows.
- No known critical or high-severity vulnerability remains without a documented
  mitigation and release decision.
- Configuration can be migrated and recovered without losing secrets or
  profiles.
- The application is usable for a full SSH, SFTP, and tunnel workflow without
  launching it from a terminal or installing an application helper process.

## 7. Test Strategy

### 7.1 Unit Tests

- Domain validation and state transitions.
- Profile and settings precedence.
- Config migrations and atomic persistence.
- Go/TypeScript bridge DTO compatibility, frontend lease/session generation
  validation, event sequencing, and cumulative acknowledgement.
- Frontend terminal-controller output queueing, ordered `onData`/`onBinary`
  input, resize coalescing, StrictMode remounting, and idempotent disposal.
- React views and state reducers without terminal-byte fixtures in React state.
- Host-key decisions and authentication ordering.
- Transfer and tunnel state machines.
- Path, address, and command construction.

### 7.2 Integration Tests

- Real local PTY and ConPTY behavior by operating system.
- PTY-to-Wails-to-xterm output and xterm-to-PTY input/resize behavior.
- Bridge ordering, backpressure, disconnect, frontend disposal, and shutdown
  during sustained output.
- Lease expiry, frontend reload/replacement, stale-generation traffic,
  cumulative acknowledgement replay, and fairness between noisy sessions.
- An isolated SSH/SFTP server fixture with deterministic host keys and accounts.
- Proxy jump, keepalive, disconnect, and cancellation.
- File transfer interruption and recovery.
- Local, remote, and dynamic tunnel traffic.
- Process-tree and connection cleanup.

### 7.3 UI Tests

- Vitest and React Testing Library for components, forms, commands, focus, and
  bridge-facing view models.
- Playwright for browser-rendered workflows, terminal interaction, layout, and
  screenshots at supported sizes and scale factors.
- Native Wails smoke tests for WKWebView, WebView2, and WebKitGTK integration.
- Native focus and traversal automation where platform tooling permits, plus
  manual keyboard, AltGr, IME, clipboard, high-DPI, and accessibility checks on
  each OS.
- Hidden-tab tests prove that selection changes neither remount controllers nor
  open or close backend runtimes.
- Terminal interaction checks using `vim`, `less`, `top`, `tmux`, and shell
  completion.

### 7.4 Reliability and Security Tests

- `go test -race ./...` on supported native runners.
- Fuzz bridge framing, config files, SSH config import, known-hosts entries, and
  remote filenames. Feed malformed terminal streams through the end-to-end
  frontend harness.
- Leak tests for goroutines, sockets, listeners, PTYs, child processes, and
  temporary files, plus frontend listener and xterm-addon disposal checks.
- Go and npm dependency and vulnerability scanning on every release.
- Security regression tests for secret serialization, host-key bypass, unsafe
  OSC handling, path traversal, accidental all-interface tunnel binding,
  unapproved bridge origins, and secret values entering frontend state or
  storage.

## 8. Performance Budgets

Initial budgets, revised only with measured evidence:

- No continuous repaint, heartbeat, or polling while no backend resource is
  active. Idle CPU is measured against a blank Wails-window baseline and may not
  show application-caused sustained work.
- Terminal output events contain at most 64 KiB raw bytes and wait at most one
  display frame before emission.
- Terminal output never causes a React component update per chunk.
- Each session has at most 1 MiB unacknowledged backend output and 1 MiB pending
  frontend terminal writes. Pressure and lease state are observable in debug
  diagnostics without recording terminal content.
- Local input echo is at most 50 ms p95 while idle and 150 ms p95 during the
  terminal-flood workload on the recorded M1 macOS reference machine.
- Input, resize, close, and lifecycle traffic cannot wait more than 100 ms
  behind bulk output; final resize delivery is at most 150 ms p95.
- Scrollback has a configurable hard line and memory cap.
- A 10 MiB terminal output burst completes within 10 seconds, does not freeze
  input, and does not grow an unbounded event queue.
- Confirmed application shutdown completes within five seconds, including
  process escalation, and leaves no child or network resource alive.
- File transfers stream through bounded buffers.
- Directory listing and filtering remain responsive at 100,000 entries through
  pagination or incremental population.
- Opening and closing repeated sessions returns goroutine and descriptor counts
  to baseline.
- Fifty open tabs, including noisy hidden tabs, stay within explicit controller,
  renderer, scrollback, and graphics-context caps.

## 9. Security Baseline

- Never use insecure SSH host-key callbacks.
- Never store passwords or passphrases in config files.
- Prefer SSH agent and OS-keychain flows. Manual secret controls never put their
  values in React state, frontend persistence, diagnostics, or telemetry; the
  unavoidable short-lived WebView string exposure is part of the threat model.
- Never log terminal contents or file contents by default.
- Never execute remote-provided terminal sequences as local commands.
- Treat OSC clipboard, hyperlinks, notifications, and title changes as untrusted
  input with explicit policies and size caps.
- Load only embedded frontend assets in production under a restrictive Content
  Security Policy without remote scripts or unsafe evaluation. Disable remote
  navigation, the default context menu, and production developer tools.
- Expose only task-oriented Go bridge methods and reject calls from unapproved
  origins.
- Validate frontend lease, session ID, session generation, payload size, and
  sequence on every runtime bridge command. Stale traffic is rejected without
  mutating the current runtime.
- Open sanitized HTTP and HTTPS links through the operating system, never inside
  the privileged application WebView.
- Bind tunnels to loopback by default.
- Use argument arrays for subprocesses. Shell interpolation is used only when a
  user explicitly configured a shell command.
- Apply private permissions to config, logs, known hosts, partial transfers, and
  exported diagnostics.
- Keep dependencies pinned, reviewed, and isolated behind internal adapters.

## 10. Risk Register and Go/No-Go Rules

1. **Wails bridge throughput:** M1 measures the packaged path. If it misses the
   accepted budgets after batching and cumulative flow control, pause feature
   work and compare another host or bridge mechanism. Do not compensate with
   unbounded buffering or a custom emulator.
2. **Frontend lease false expiry:** M1 stress-tests main-thread stalls,
   minimized windows, and system sleep/wake, then records a suspend-safe grace
   policy that retains bounded orphan cleanup.
3. **Windows WebView behavior:** M2 tests focus, AltGr, IME, and tab traversal in
   a native build before workspace expansion. If supported APIs cannot provide
   reliable behavior, narrow Windows support or reconsider the host before M3.
4. **WebView secret exposure:** M5 either records acceptance of minimized
   transient exposure or implements a native prompt before interactive SSH.
5. **Linux runtime variation:** M2 and M5 test documented supported
   distributions natively; M11 publishes an explicit WebKitGTK, Secret Service,
   and packaging support matrix with prerequisite diagnostics.
6. **Cross-platform scope:** Work remains macOS-first inside a milestone, but no
   milestone claims parity without its native gates. X11, RDP, VNC, Mosh,
   serial, and legacy encodings remain outside 1.0.

The product remains a sane project only while M1 proves the bridge and M2
proves native input/lifecycle behavior. Those are genuine stop-and-reconsider
gates, not paperwork that can be waived to keep adding features.

## 11. Delivery Order and Estimated Size

These are solo-engineering estimates after the current scaffold, not calendar
commitments:

| Milestone | Expected effort | Release value |
| --- | ---: | --- |
| M0 Foundation | 4-7 days | Wails migration and honest lifecycle |
| M1 Terminal proof | 7-12 days | Proven PTY/bridge/xterm vertical slice |
| M2 Local terminal | 8-14 days | First genuinely usable program |
| M3 Workspace | 4-7 days | Reliable multi-session desktop UX |
| M4 Profiles/config | 4-7 days | Durable daily workflow |
| M5 Credentials/trust | 5-8 days | Secure SSH foundation |
| M6 SSH | 7-12 days | Primary remote workflow |
| M7 SFTP | 8-14 days | File workflow |
| M8 Tunnels | 6-10 days | Network workflow |
| M9 Productivity | 6-10 days | Operator efficiency |
| M10 UX/settings | 5-9 days | Product completeness |
| M11 Packaging | 5-10 days | Installable native releases |
| M12 Hardening | 7-14 days | 1.0 release confidence |

A credible core 1.0 is roughly 16-27 focused engineering weeks for one person,
depending mainly on bridge performance, Windows behavior, and cross-platform
credential integration. Using xterm.js removes the custom terminal-emulator
project but does not remove PTY, lifecycle, security, or platform work. Embedded
X11, RDP, and VNC are additional major projects, not small checkboxes.

## 12. Working Rules for Implementation

For every milestone:

1. Write or update the domain contract and acceptance tests first.
2. Update the typed Go/TypeScript bridge contract when the workflow crosses the
   desktop boundary.
3. Implement the backend adapter with no Wails or frontend imports.
4. Add the use-case orchestration and deterministic cleanup path.
5. Build the complete React workflow, including loading, empty, error, retry,
   and close states. Terminal bytes stay outside React state.
6. Run Go and frontend unit, integration, race, Playwright, and native smoke
   tests as relevant.
7. Verify the packaged application manually on the current OS.
8. Run the milestone's native platform gates; browser tests and cross-compiles
   do not substitute for a WKWebView, WebView2, or WebKitGTK run.
9. Update architecture decisions, benchmark records, and user documentation.
10. Remove replaced placeholder code before declaring the milestone complete.

The immediate implementation sequence is M0, M1, then M2. No SFTP, tunnel, or
settings surface should expand until a local terminal can start, interact,
resize, exit, and clean up correctly.
