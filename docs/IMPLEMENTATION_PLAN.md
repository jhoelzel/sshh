# Implementation Plan

## 1. Implementation Status

The macOS core feature slices through M9, plus the terminal,
connection-policy, resumable-transfer, transfer-policy, and notification
portions of M10, are implemented. The repository contains a Wails v2 host, an
embedded React and strict TypeScript frontend, xterm.js terminal controllers,
a real Unix PTY adapter, strict SSH and known-host adapters, SFTP, saved tunnel
state, and lease-owned runtime managers with bounded bridge flow control.

Implemented and verified:

- [x] No shell starts with the application; local PTYs start only from an explicit
  profile action and always have a visible tab.
- [x] Session IDs, generations, and renewable frontend leases reject stale bridge
  traffic and close resources after frontend loss.
- [x] PTY output is held until the xterm controller activates, sent as ordered
  chunks, bounded by a cumulative acknowledgement window, and delivered through
  a manager-owned fair output scheduler that control and lifecycle traffic
  bypasses.
- [x] Terminal input is ordered and idempotent, resize is validated and debounced,
  and close escalates from hangup to termination to kill within bounded time.
- [x] The macOS arm64 application launches as a self-signed `.app` with all
  frontend assets embedded in its Go executable.
- [x] Profiles use a strict versioned schema, atomic private-file replacement,
  external-change detection, and legacy migration backup. Local profiles expose
  validated environment overrides with exact value preservation.
- [x] Profile exchange uses a versioned credential-field-free JSON document,
  private atomic exports, and one-save imports. Concrete OpenSSH hosts import
  with first-value precedence and visible diagnostics; unsafe proxy directives
  are skipped rather than converted into unintended direct connections.
- [x] One-off SSH quick connect creates only a validated transient profile. It
  reuses strict host-key and authentication workflows and can open a terminal
  without writing profile configuration.
- [x] SSH supports explicit first-use trust, changed-key rejection, agent, key,
  password, and keyboard-interactive authentication without persisting secrets.
- [x] Connection policy uses a migrated validated settings schema. Configurable
  deadlines bound host-key probes, TCP connection, and SSH handshake setup;
  connection-group-owned keepalives detect unanswered transports for terminals,
  SFTP, and tunnels without opening any connection at application startup.
- [x] Reference-counted SSH connection groups share one authenticated client across
  overlapping terminal, SFTP, and tunnel leases. Concurrent first opens use one
  dial, feature closes remain isolated, and the final lease closes the client,
  keepalive coordinator, and connection waiter.
- [x] SFTP operations stream through a live-configurable bounded worker pool and
  atomic partial files. Ask, overwrite, skip, and keep-both collision policies
  are enforced by the backend with in-flight destination reservations; the file
  workspace exposes navigation, upload, download, rename, delete, mkdir, chmod,
  progress, and cancellation.
- [x] Opt-in resumable uploads and downloads persist private versioned metadata
  before copying, survive application restart, and expose only explicit Resume
  and Discard actions inside a matching visible SFTP workspace. Upload resume
  verifies source, prefix, and completed-partial SHA-256 integrity; download
  resume validates remote metadata, partial type and size, and file identity
  throughout the resumed operation.
- [x] Profile favorites sort saved connections first. Remote-path favorites use a
  separate strict private store keyed by saved SSH profile and expose explicit
  star-toggle and quick-navigation controls inside an open SFTP workspace.
- [x] Local, remote, and SOCKS5 tunnels have saved independent models, loopback
  defaults, guarded public binds, lifecycle events, bounded relays, cancellation,
  retry, and explicitly enabled auto-start.
- [x] Command snippets use a strict private store, validated variables, an exact
  backend-rendered preview, explicit live targets, and confirmation for
  multi-terminal execution.
- [x] Session logging is opt-in, output-only, privately stored, timestamp-capable,
  bounded by rotation, visible while active, and owned by terminal shutdown.
- [x] Terminal defaults for font, size, spacing, cursor, scrollback, and bell use a
  validated versioned store, apply live to open controllers, and reset
  durably.
- [x] Native notifications are disabled by default, request OS authorization only
  from an explicit Settings action, and can report long completed transfers or
  failed terminal sessions. Preferences migrate through the versioned settings
  store; payloads are bounded and omit full paths, contents, and credentials.
- [x] The command palette searches grouped connection, profile, navigation, and
  active-terminal actions. Arrow-key operation and non-conflicting
  `Cmd/Ctrl+Shift` shortcuts work without intercepting ordinary shell input.
- [x] Terminal text actions copy the current xterm viewport through the native
  clipboard and export exact selections through a native Save dialog. Export
  names are sanitized and files are bounded, private, synced, and atomically
  replaced.
- [x] Workspace layouts use a strict versioned private store and retain only
  ordered profile references, display snapshots, and the selected index.
  Restore creates disconnected frontend tabs with no process or network
  resource; each tab reconnects explicitly through the normal trust and
  credential workflow.
- [x] Native builds embed version, source revision, UTC build date, and dirty-tree
  state. Settings exposes that identity with the Go version and target platform;
  direct Go builds fall back to embedded VCS metadata.
- [x] Read-only GitHub Actions run Go and frontend quality gates, race tests,
  call-graph and npm vulnerability scans, and native macOS, Linux, and Windows
  Wails compiles. Actions are pinned to immutable SHAs and dependencies receive
  weekly update checks.
- [x] Go race tests cover managers and adapters. Real loopback integration tests
  cover PTY binary input and live resize, shared multi-channel SSH terminal
  lifetime, terminal exit, repeated flood-close process-tree cleanup, 100-cycle
  short-lived PTY churn, SFTP operations, and bidirectional local, remote, and
  SOCKS forwarding. Controller tests cover mixed text, binary mouse, and paste
  ordering, resize coalescing, malformed frames, the real xterm parser, and
  listener disposal during output. TypeScript, ESLint, Vitest, vet, and
  production builds pass.
- [x] A guarded packaged-macOS WKWebView harness measures the complete PTY,
  bridge, xterm, input, resize, and close path without recording terminal
  content. The accepted 10 MiB run records hardware, queue high-water marks,
  p95 latency, completion time, and app/fixture/WebKit RSS.
- [x] A separate guarded 15-minute packaged-macOS soak keeps eight PTYs and
  xterm controllers active, rotates the visible tab, records 1,440 input echoes,
  moves 439.5 MiB, proves exact queue drain and cleanup, and bounds warmed RSS
  growth to 18.2 MiB.

Still required for the complete cross-platform and 1.0 gates:

- [ ] ConPTY support and Windows validation.
- [x] Native macOS throughput and memory measurements for the 10 MiB terminal
  path, including deterministic scheduler fairness and explicit queue,
  scrollback, latency, close, and whole-application RSS budgets.
- [x] Long-run terminal soak and multi-session native stress measurements on the
  current Apple Silicon Mac. Multi-hour, larger-count, Linux, and Windows runs
  remain separate gates.
- [ ] Accessibility-driven native interaction automation or a dedicated Wails E2E
  harness. macOS launch and frontend attachment are verified today; runtime
  behavior is also exercised below the WebView boundary.
- [ ] Remaining reconnect, proxy, known-hosts, and agent settings.
- [ ] Signed/notarized macOS releases, Linux packaging validation, Windows WebView2
  runtime and ConPTY implementation, accessibility review, and broader
  cross-platform and multi-hour performance evidence.

## 2. Product Definition

### 2.1 Core 1.0

Version 1.0 will be a production-usable MobaXterm-style core client with:

- [x] A native desktop workspace with searchable profiles and multiple session
  tabs.
- [x] Interactive local shells using a real pseudoterminal on macOS and Linux.
- [x] Interactive SSH shells with password, keyboard-interactive, private-key, and
  SSH-agent authentication.
- [x] Strict SSH host-key verification and an explicit first-connection trust flow.
- [x] A usable xterm-compatible terminal with colors, Unicode, scrollback,
  selection, clipboard, keyboard shortcuts, resize, alternate screen, and mouse
  reporting.
- [x] UTF-8 terminal sessions in 1.0. Legacy character-set conversion is a
  post-1.0 feature and is never guessed silently from terminal output.
- [x] Profile create, edit, duplicate, delete, import, group, tag, local
  environment, and quick-connect workflows.
- [ ] Add a dedicated local file pane beside the implemented remote SFTP browser
  and visible transfer queue.
- [x] Saved local, remote, and dynamic SSH tunnels with explicit lifecycle state.
- [x] Session logs, snippets, and guarded multi-session input.
- [ ] Complete settings for shell, terminal, appearance, connection behavior,
  and data locations.
- [ ] Native macOS, Linux, and Windows builds from one Go codebase. macOS arm64
  is the currently verified native package target.
- [ ] One self-contained application package per target. Frontend assets are
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

### 2.4 Optional Teleport Integration Track

A post-1.0 Teleport track may register administrator-managed Teleport Proxies,
authenticate through the cluster's external-browser, SSO, and MFA flows,
discover authorized SSH nodes and leaf clusters, save target shortcuts, and
open `tsh ssh` sessions through the existing terminal lifecycle.

The initial design requires a separately installed, user- or
administrator-provided `tsh` on the computer running `shh-h`. It does not
bundle, download, update, or redistribute the Teleport client, and it does not
install or administer Teleport services on target infrastructure. Teleport
clusters, dynamic resources, short-lived identities, access requests, and
recording policy remain distinct from ordinary SSH profiles and host-key trust.

This track changes the self-contained-runtime promise and introduces external
binary, license, TLS, SSO callback, certificate lifecycle, policy, and session
recording boundaries. It requires dedicated ADRs and release gates. The
proposed architecture, phased delivery plan, threat model, and acceptance
criteria are in `docs/TELEPORT_INTEGRATION_PLAN.md`.

## 3. Non-Negotiable Behavior

### 3.1 Session Visibility and Background Work

- [x] Starting the application starts no shell and opens no network connection.
- [x] A local process starts only after the user chooses New Local Terminal or
  connects a local profile.
- [x] An SSH connection starts only after the user explicitly connects a profile.
- [x] Restoring a saved workspace creates disconnected tabs only; it never opens a
  PTY, SSH transport, SFTP session, tunnel, or other network resource.
- [x] Every running terminal has a visible tab and state indicator.
- [x] An unfocused tab may continue running, as normal terminal tabs do, but it is
  never invisible background work.
- [x] A live Go runtime belongs to one frontend lease and one session generation.
  Stale commands and events from an older frontend or session generation are
  rejected.
- [x] A frontend reload, crash, or replacement cannot leave an unowned process or
  connection running. While live resources exist, a lightweight frontend lease
  is renewed; losing it starts bounded shutdown unless a deliberately designed
  reattachment protocol is added later.
- [x] Closing a running tab prompts when needed, terminates its process or SSH
  channel, waits for completion, and removes it from the runtime registry.
- [x] Closing the main window presents one consolidated confirmation if sessions,
  transfers, or tunnels are active. Confirming shutdown stops all of them.
- [x] The application has no tray-only mode and leaves no daemon or child shell
  behind after normal exit.
- [x] Only one application instance owns the runtime and writable configuration.
  A secondary launch focuses the existing window and passes it any supported
  launch request.
- [ ] Tunnels and transfers may continue while another tab is selected, but they
  remain visible in a global Activity view.

### 3.2 Failure Behavior

- [ ] Failed connections become visible failed sessions with a useful reason and a
  Retry action.
- [x] Network loss never pretends that a session is connected.
- [x] Frontend loss is a lifecycle failure, not an invitation to buffer terminal
  output indefinitely. Backend and bridge queues remain bounded while the
  frontend lease expires, then all lease-owned resources close.
- [x] Development hot reload follows the same ownership rule. Version 1.0 closes
  active runtimes on frontend replacement instead of attempting an unsafe
  implicit reattachment.
- [x] Terminal sessions are not silently resumed because an arbitrary shell cannot
  be resumed safely. Reconnect opens a new channel; tmux or screen remains the
  user's explicit persistence mechanism.
- [x] Partial file downloads use a temporary name and are never presented as
  complete files.
- [x] A host-key mismatch is a hard failure, not a warning that can be casually
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
  control. The manager also owns one bounded round-robin output dispatcher for
  all live sessions.
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

Each output pump may have only one chunk in the dispatcher at a time. The
dispatcher serializes sink delivery and rotates session IDs after each chunk;
its bounded ingress queue applies backpressure if many sessions publish at
once. Input, resize, close, and lifecycle state use their direct manager paths,
so they do not enter or wait behind the output queue. Stopping the last session
signals the dispatcher without waiting for an in-flight GUI callback. Every
event is checked against its frontend lease and session generation before
delivery.

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

- `OnStartup` records only the Wails context. It never opens stores, runs
  migrations, constructs services, or starts runtime-owned goroutines because
  Wails v2 invokes this hook before its Linux single-instance setup.
- The first `OnDomReady`, which occurs after native single-instance resolution
  on every supported platform, composes backend services exactly once and marks
  the host ready. React waits on the typed `AwaitReady` bridge command before it
  imports the product application or begins the frontend attachment handshake.
- Startup failure releases the same readiness wait with a cause-redacted typed
  error and closes any partially composed runtime. Lifecycle control methods
  remain on a host-only controller and are not callable through Wails bindings.
- `OnBeforeClose` owns close interception. With active resources it prevents
  the first close, requests one consolidated frontend decision, and permits a
  programmatic close only after coordinated backend shutdown completes.
- `OnShutdown` runs after the frontend is gone. It is a final idempotent cleanup
  and assertion path, never the first signal on which frontend acknowledgement
  depends.
- Wails `SingleInstanceLock` is enabled from M0. A secondary instance sends a
  bounded launch request to the primary instance, focuses it, and exits before
  DOM-ready, so it cannot open config stores or runtimes. A callback received
  before primary context preparation is queued and delivered after preparation.
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

Progress notation:

- [x] Implemented and supported by code, tests, or recorded macOS verification.
- [ ] Outstanding, only partially implemented, or still missing its stated exit
  evidence. A milestone remains open until every required native gate passes.

Checklist reconciliation rule: every implementation slice updates its matching
deliverable, exit-gate evidence, implementation snapshot, and milestone status
in the same change. A box stays open when any clause or required evidence is
still incomplete.

### M0: Foundation and Engineering Gates

Deliverables:

- [x] Record the accepted Wails v2, React/TypeScript, and xterm.js decision in an
  ADR, including alternatives and the M1 validation gate.
- [x] Preserve backend behavior with tests, scaffold the Wails host and frontend,
  then remove Fyne and its transitive dependencies.
- [x] Pin an exact stable Go toolchain, Node 24 LTS release, npm, Wails CLI, Go
  dependencies, and frontend dependencies. Configure strict TypeScript, React
  StrictMode, linting, formatting, frontend unit tests, `package-lock.json`, and
  reproducible `npm ci` builds.
- [x] Generate and validate Go-to-TypeScript bridge contracts in CI through a
  production-mode macOS Wails build and deterministic generated-file comparison.
- [x] Record process lifecycle, SSH trust, secrets, WebView security, and
  single-application distribution decisions.
- [x] Replace the current generic backend package shape with domain, use-case,
  port, and adapter boundaries as code is touched.
- [x] Introduce a consistent typed error taxonomy. Go use cases and adapters use
  stable codes, Wails formats every rejection into a cause-redacted JSON
  envelope, and the frontend normalizes it into `BackendError` without parsing
  human-readable messages.
- [x] Establish automated `govulncheck`, high-severity npm audit, weekly Go/npm/
  Actions dependency updates, Go test/race/vet, TypeScript, Vitest, ESLint,
  frontend production, and native macOS, Linux, and Windows build checks.
- [x] Add build metadata: semantic version, commit, build date, and dirty state.
- [x] Add a root application context and coordinated shutdown service.
- [x] Configure `OnStartup`, `OnDomReady`, `OnBeforeClose`, and `OnShutdown` with
  idempotent ownership. Keep `OnStartup` inert, resolve `SingleInstanceLock`
  before one-time DOM-ready composition and writable config migration, and gate
  React startup on typed backend readiness.
- [x] Lock bridge origins to embedded content, disable production developer tools
  and the default context menu, block remote navigation, and verify that the
  production build starts no HTTP listener.
- [x] Remove fake active-session counts and the example.com default profile.
- [x] Keep only actions that perform real work; unfinished features remain absent
  rather than showing success-like placeholders.

Tests and exit gate:

- [x] Current config behavior is covered before migration.
- [x] The application starts and exits without spawning a child process.
- [x] Repeated startup and shutdown leave no lease-monitor goroutine behind.
  Twenty-five in-process lifecycle cycles and repeated-start replacement are
  covered; configuration stores retain no open file handles between operations.
- [x] A second application launch focuses the first instance and exits without
  opening a second writable config store. Unit coverage proves options and
  `OnStartup` perform no composition, DOM-ready composition is one-time, and
  early/direct activation callbacks focus the prepared primary. A packaged
  arm64 macOS two-process smoke on 2026-07-17 recorded an immediate zero-status
  secondary exit, one surviving host PID, unchanged config metadata, and the
  existing `shhh` process restored to the foreground.
- [x] React StrictMode's development effect replay does not duplicate bridge
  listeners, controllers, commands, or backend resources. Nonce-scoped
  in-flight loaders coalesce startup commands, listeners use Wails'
  subscription-specific disposers, and same-nonce backend attachment is
  idempotent. A rendered whole-App gate proves one startup command set, one
  active listener set, one terminal command and controller per keyboard action,
  complete cleanup, and a fresh single command set on a genuine later remount.
- [x] The Wails application builds and launches as an arm64 `.app` on the current
  Mac using the system WKWebView.
- [x] Native macOS, Linux, and Windows CI jobs build the desktop shell. Ubuntu
  uses WebKitGTK 4.1 through Wails' `webkit2_41` build tag, while Windows keeps
  the explicit unsupported ConPTY adapter until M2; no stub can report a
  successful terminal operation.
- [x] Production assets load from the embedded filesystem without a runtime Node
  process or local HTTP server.

### M1: Wails and xterm.js End-to-End Terminal Proof

Deliverables:

- [x] Pin a stable xterm.js release and the stable fit and search addons.
- [x] Build a `TerminalController` that owns xterm outside React state and disposes
  every listener and addon deterministically. React only attaches its persistent
  host element and never opens a runtime from a mount effect.
- [x] Implement the typed Wails commands and lifecycle/output events for a single
  diagnostic terminal.
- [x] Implement the frontend attachment lease, active-resource-only renewal, lease
  expiry, and session generation checks. Frontend replacement closes the
  diagnostic runtime within the shutdown budget.
- [x] Implement a minimal Darwin PTY adapter sufficient for a real local shell
  spike, with explicit process cleanup.
- [x] Preserve raw PTY bytes through output events containing lease ID, session ID,
  generation, sequence, raw byte count, byte-safe payload, and final marker.
- [x] Implement a cumulative byte-credit window with bounded batch sizes,
  frontend/backend queues, and transport backpressure.
- [x] Connect both xterm `onData` and `onBinary` through one ordered input queue.
  Add monotonic input sequence numbers, bounded in-flight writes, large-paste
  chunking, and debounced/coalesced resize without routing bytes through React
  state.
- [x] Prioritize input, resize, close, and lifecycle traffic and fairly schedule
  output across diagnostic sessions used by the stress harness. Manager tests
  block output delivery while exercising each control path; a deterministic
  10 MiB scheduler test verifies round-robin service for a low-volume peer.
- [x] Implement fit, scrollback, selection, copy, bracketed paste, search, focus,
  terminal title, and bell indication using stable public APIs.
- [x] Use xterm's standard renderer and keep WebGL disabled by default.
- [ ] Evaluate WebGL only as an optional measured acceleration path with
  automatic fallback.
- [ ] Allow only sanitized HTTP and HTTPS links to open through the operating
  system.
- [x] Disable remote navigation and OSC 52 clipboard writes by default.
- [x] Add a production Content Security Policy and disable production developer
  tools and unneeded WebView capabilities.

Tests and exit gate:

- [x] The real shell supports typing, Ctrl+C, resize, paste, selection, copy,
  scrollback, and clean exit on the current Apple Silicon Mac.
- [ ] `vim`, `less`, `top`, `tmux`, color output, Unicode, emoji, combining marks,
  and IME input render and behave correctly.
- [x] Sequence tests prove no duplicated, reordered, or silently dropped chunks.
- [x] Duplicate cumulative acknowledgements are harmless; stale commands and late
  events from an old generation or frontend lease are rejected or discarded.
- [x] Interleaved `onData`, `onBinary`, and paste input reaches a real PTY in
  callback order, including binary mouse reports, while coalescing delivers the
  final resize. Controller, bridge-to-transport, and Darwin/Linux PTY tests
  cover each boundary.
- [x] Invalid UTF-8 and malformed terminal streams do not crash Go, Wails, or the
  WebView. Raw-byte manager and DTO tests, bounded controller validation, parser
  failure containment, and the real xterm parser test cover the complete path.
- [x] A 10 MiB output burst remains interactive and memory stays within explicit
  queue and scrollback caps and passes the provisional latency, fairness, and
  completion budgets in section 5.3. The packaged arm64 macOS run and hardware
  record are in `docs/TERMINAL_BENCHMARK.md` and its machine-readable report.
- [x] The same flood in one diagnostic session cannot starve input, resize,
  close, lifecycle events, or a second low-volume diagnostic session at the
  manager scheduler boundary. Native WebView latency and memory measurements
  remain part of the preceding open performance gate.
- [x] Repeated React StrictMode attach/detach cycles and a frontend reload create no
  duplicate shell and leave no old-lease shell running. Whole-App tests prove
  one command, controller, and listener set per genuine mount; serialized
  backend attachment waits for a live old-lease runtime to close before returning
  the replacement lease.
- [ ] Minimizing the window, a deliberate main-thread stall, and system sleep/wake
  do not expire a healthy frontend lease. Simulated frontend loss does expire it
  and reap the shell within the measured bounded grace period.
- [x] Closing the tab or window during heavy output reaps the shell and returns
  goroutine, descriptor, and bridge-listener counts to baseline. A Darwin/Linux
  real-PTY test repeats both close paths after 2 MiB floods, verifies descendant
  process-group cleanup and zero runtimes, and compares process-wide counts with
  warmed baselines. The whole-App test proves tab controller disposal and root
  listener cleanup while output events are arriving.
- [x] `OnBeforeClose` performs the visible decision and coordinated shutdown;
  `OnShutdown` remains safe when invoked after frontend destruction.
- [x] A packaged arm64 macOS `.app` passes the launch and frontend-attachment
  smoke workflow; the broader TUI and stress matrix remains open above.

If this end-to-end gate fails, implementation stops before broader UI work and
the host/bridge design is revisited using the measurements. It does not fall
back to a hand-built terminal emulator by default.

### M2: Real Local Terminal Vertical Slice

Deliverables:

- [x] Implement the common `TerminalTransport` contract.
- [x] Productionize Unix PTY startup, input, output, resize, signal, wait, and
  process-group cleanup for Darwin and Linux builds.
- [ ] Add native Linux PTY and WebKitGTK coverage.
- [ ] Implement Windows ConPTY startup, input, output, resize, wait, and cleanup.
- [x] Add local shell discovery and profile options for executable, arguments,
  working directory, and login-shell behavior.
- [x] Expose portable local-profile environment overrides in an accessible
  key/value editor, with frontend and authoritative domain validation, exact
  persistence, and real-PTY delivery coverage.
- [x] Wire explicit New Local Terminal and Connect actions to a live runtime.
- [x] Replace the scaffold terminal label with reusable xterm-backed session tabs.
- [x] Update terminal title and visible exit status from runtime metadata.
- [ ] Add running duration and current-directory reporting when safely available.
- [x] Make Ctrl+C, Ctrl+D, Ctrl+Z, resize, paste, and full-screen TUI applications
  behave correctly.
- [x] Implement close escalation: request graceful exit, wait briefly, terminate the
  process group, then force kill only if required.
- [x] Always reap the child process.

Tests and exit gate:

- [x] Extend the real PTY integration coverage, which now includes binary input,
  live resize, output, initial size, exit status, and deterministic child
  process-tree termination after a descendant ignores hangup.
- [x] Closing a tab returns process, file descriptor, goroutine, and runtime
  counts to baseline under a repeated Darwin/Linux real-PTY leak test.
- [x] Closing the application with a running shell follows the confirmation and
  cleanup contract.
- [x] Opening 100 short-lived terminals in a real Darwin/Linux PTY test loop
  returns manager runtimes, dispatchers, goroutines, and descriptors to the
  warmed baseline, including under the Go race detector.
- [x] No shell starts on application launch or mere profile selection.
- [ ] Native Windows tests cover WebView focus restoration, forward and reverse tab
  traversal, AltGr, IME composition, clipboard shortcuts, ConPTY resize, and
  close during output; browser-only tests do not satisfy this gate.
- [ ] Native Linux tests cover WebKitGTK focus, clipboard, PTY resize, process-group
  cleanup, and the documented minimum runtime version.

This is the first production-quality milestone that turns the proof into a real
cross-platform program.

### M3: Session Workspace and Tab Lifecycle

Deliverables:

- [x] Add closeable session tabs with protocol and state indicators.
- [ ] Add session-tab reordering.
- [x] Add keyboard-operated new-terminal commands and focus restoration.
- [ ] Add tab search and explicit keyboard tab-navigation commands.
- [x] Support multiple simultaneous terminal runtimes without shared mutable xterm
  controllers or React component state.
- [x] Keep one persistent terminal host/controller per open tab. Hidden tabs retain
  terminal state without repaint work, are not unmounted by ordinary tab
  selection, and refit when made visible.
- [x] Cap scrollback through validated settings and keep WebGL disabled.
- [ ] Add an explicit open-session resource cap and test any future visible-pane
  WebGL context cap.
- [x] Add clear starting, running, disconnected, failed, exited, and closed states.
- [x] Add Close and explicit reconnect for restored disconnected tabs.
- [ ] Add Retry, Reconnect in New Tab, Duplicate Tab, Clear Scrollback, and Reset
  Terminal actions.
- [ ] Add split-terminal layout only after tabs are stable.
- [ ] Add a global Activity view for sessions, transfers, and tunnels.
- [x] Add coordinated shutdown and consolidated close confirmation.
- [ ] Persist window geometry, sidebar width, selected theme, and non-sensitive UI
  preferences. Never attempt to resurrect dead processes on restart.

Tests and exit gate:

- [ ] Add exhaustive state-machine transition rejection tests.
- [ ] Add a concurrent tab open/output/resize/close race scenario; the existing
  full Go suite passes the race detector.
- [ ] React component tests and Playwright flows cover focus, shortcuts, tab close,
  split layout, and shutdown decisions.
- [ ] Native Wails smoke tests cover window close interception and lifecycle hooks.
- [ ] Add measured coverage proving an inactive persistent tab consumes no redraw
  work unless its model changes.
- [ ] Stress tests cover at least 50 open tabs, sustained output in several hidden
  tabs, repeated active-tab changes, and deterministic controller disposal.

### M4: Profiles, Configuration, and Migration

Deliverables:

- [x] Replace the bare profile array with a versioned config document.
- [x] Add deterministic migrations and backup-before-migration behavior.
- [x] Preserve atomic writes, add file sync where supported, and enforce private
  permissions.
- [x] Add profile CRUD, duplicate, folders/groups, tags, favorites, sorting, and
  filtering.
- [x] Add quick connect without requiring a saved profile.
- [x] Add protocol-specific forms that show only relevant fields.
- [x] Add SSH config import for hosts, user, port, and identity files with OpenSSH
  first-value precedence. Proxy and other unsupported directives are reported;
  connection-critical options that cannot be represented skip the affected
  host rather than silently changing its route.
- [x] Add import/export that deliberately excludes runtime IDs, timestamps, and
  dedicated credential fields.
- [ ] Add terminal-display defaults per profile with global fallbacks.
- [x] Detect external file changes or conflicting edits before overwriting
  configuration. Process-level single-instance ownership is already enforced
  from M0.

Tests and exit gate:

- [x] Every implemented schema migration has a forward fixture.
- [ ] Corrupt and truncated config files produce a recoverable UI flow with backup
  options.
- [x] Duplicate IDs and names do not overwrite profiles.
- [ ] Expand profile validation tests beyond current host, port, name, IPv6, and
  environment coverage to shell paths, proxy chains, and invalid combinations.

### M5: Credentials and SSH Trust

Deliverables:

- [ ] Implement the cross-platform secret-store port.
- [x] Add session-only password/passphrase prompts and SSH-agent authentication.
- [ ] Add remember-in-keychain credential choices and keychain-first lookup.
- [x] Prefer SSH agent and usable private keys before a manual password prompt.
- [x] Parse unencrypted and encrypted OpenSSH private keys.
- [ ] Move key-passphrase, password, and keyboard-interactive entry out of React
  state. Prompts work today, but the native-prompt/string-lifetime gate remains.
- [x] Read the user's OpenSSH known-hosts file and maintain an application-specific
  known-hosts file.
- [x] Show algorithm and SHA-256 fingerprint on first contact.
- [x] Require explicit trust for a new host and distinguish permanent from
  session-only trust.
- [x] Treat changed host keys as hard failures with explanatory details.
- [ ] Add explicit revoked-key classification and coverage.
- [ ] Add safe diagnostic logging with host and profile IDs but no passwords,
  passphrases, private keys, terminal contents, or file contents.

Tests and exit gate:

- [ ] Mock secret stores cover unavailable, locked, denied, and deleted secrets.
- [ ] Expand known-host tests beyond known, unknown, changed, and lease-bound trust
  to revoked, hashed, IPv6, and non-default-port entries.
- [ ] Add explicit authentication-cancellation cleanup coverage.
- [x] Serialized profile and portable exchange schemas contain no credential
  material.
- [ ] Frontend tests prove secret controls never dispatch values to application
  state or persistence. A threat-model review explicitly accepts the WebView
  string-lifetime limitation or requires a native prompt before M6.

### M6: Interactive SSH

Deliverables:

- [x] Dial with `net.Dialer` and context deadlines, then perform the SSH handshake.
- [x] Support password, keyboard-interactive, private key, agent, and ordered
  fallback authentication.
- [x] Request an `xterm-256color` PTY with the terminal's actual dimensions.
- [x] Forward terminal input and output through the common runtime.
- [x] Send SSH window-change requests after UI resize.
- [ ] Support startup directory and startup command without unsafe string
  concatenation.
- [x] Add configurable keepalives and server-alive failure thresholds. Implemented
  with a global validated policy captured by each new SSH connection, bounded
  unanswered requests, and connection-context cleanup.
- [ ] Add proxy jump through one or more SSH profiles with loop detection.
- [ ] Add optional agent forwarding with a prominent per-profile opt-in.
- [x] Make disconnect reasons and remote exit status visible.
- [x] Introduce SSH connection groups and leases for terminal, SFTP, and tunnel
  reuse. Implemented with keyed concurrent-dial serialization, per-feature
  close signals, final-reference teardown, remote-close eviction, and bounded
  application shutdown.

Tests and exit gate:

- [x] Real SSH integration covers password authentication, terminal round-trip,
  PTY resize/exit, and shared multi-channel connection lifetime.
- [ ] Expand integration fixtures to every authentication method, host-key trust,
  Unicode, connection timeout, abrupt disconnect, and cancellation.
- [ ] Add proxy-jump lifecycle tests that close every hop in reverse order.
- [x] Connection-pool tests cover final-lease close, canceled waiters, concurrent
  first dial, remote-close eviction, keepalives, and shutdown.
- [x] No code path uses `ssh.InsecureIgnoreHostKey`.
- [x] Closing the final lease closes the socket and waiter goroutines.

### M7: SFTP Browser and Transfer Manager

Deliverables:

- [x] Implement remote filesystem operations through a narrow filesystem port.
- [x] Add a remote pane with path navigation, sorting, hidden-file toggle,
  refresh, profile-scoped bookmarks, and keyboard operation.
- [ ] Add a dedicated local filesystem pane.
- [x] Add create directory, rename, delete with confirmation, chmod, upload, and
  download actions.
- [ ] Add open-with-system actions.
- [x] Add a transfer manager with bounded concurrency, queueing, progress,
  cancellation, and per-transfer errors.
- [ ] Add transfer speed, ETA, and retry controls beyond persisted resume.
- [x] Download to a temporary partial file and atomically rename on success.
- [x] Support resumable upload/download where server capabilities and metadata make
  it safe. Implemented with private versioned records, explicit Resume/Discard,
  deterministic partial paths, source validation, exclusive destination
  reservations, and upload prefix/final checksum verification.
- [x] Define symlink behavior explicitly and prevent accidental recursive cycles.
- [x] Keep transfers alive when a terminal tab closes only while their SSH
  connection lease and visible transfer item in the matching SFTP workspace
  remain.

Tests and exit gate:

- [x] In-process SFTP and manager tests cover listing, offset I/O, rename,
  symlink rejection, interruption, restart resume, cancellation, and checksum
  comparison.
- [ ] Add Unicode-name and remote-permission round-trip coverage to the real SFTP
  fixture.
- [x] Local and remote destination collision policies are explicit: ask, overwrite,
  skip, or rename. Implemented with native ask dialogs, visible skipped states,
  and deterministic numbered destination reservations.
- [x] Transfer queue memory is bounded and files are streamed through fixed-size
  buffers, never loaded fully into memory.
- [ ] Add a large-file integration fixture and memory-budget assertion.
- [x] Disconnecting during transfer produces a resumable failed item, not a false
  success.

### M8: SSH Tunnels

Deliverables:

- [x] Add saved tunnel models independent of terminal profiles.
- [x] Implement local forwarding, remote forwarding, and dynamic SOCKS5 forwarding.
- [x] Add bind address, requested port, destination, profile, startup policy, and
  reconnect policy.
- [x] Default local bind addresses to loopback and require confirmation before
  binding all interfaces.
- [x] Show starting, active, retrying, failed, and stopped states with actual bound
  addresses.
- [x] Handle every accepted connection in a cancellable child context.
- [x] Use SSH connection leases so stopping a tunnel does not disrupt unrelated
  channels.
- [x] Add explicit Start, Stop, Restart, Edit, and visible error actions.

Tests and exit gate:

- [x] Local, remote, and SOCKS forwarding pass bidirectional integration tests.
- [x] Port collision, reconnect after network failure, cancellation, and secret
  reconnect restrictions are covered.
- [ ] Add explicit denied remote-forwarding and DNS-failure fixtures.
- [x] Stopping a tunnel closes its listener and every relayed connection.
- [x] No tunnel auto-starts unless the user explicitly enabled that tunnel.

### M9: Productivity Features

Deliverables:

- [x] Add reusable command snippets with folders, tags, variables, and a preview
  before execution.
- [x] Add guarded multi-execution mode with a persistent visual warning and an
  explicit target-session list.
- [x] Add optional session logging with start/stop controls, timestamp policy,
  rotation, and secure file permissions.
- [x] Add terminal search, copy-all-visible, and export-selection actions.
  Implemented for the active terminal through the toolbar and command palette.
- [x] Add saved workspace layouts that restore profile tabs as disconnected tabs;
  reconnection remains explicit. Implemented for saved-profile terminal tabs.
- [x] Add profile and remote-path favorites. Implemented with profile sorting and
  profile-scoped canonical path navigation that starts no connection.
- [x] Add command palette and consistent keyboard shortcuts.
- [x] Add notifications for long transfer completion and unexpected disconnect,
  respecting OS and application settings. Implemented with a native Wails
  adapter, explicit permission flow, saved category controls, a duration
  threshold, and bounded privacy-preserving payloads.

Tests and exit gate:

- [x] Snippet variables are sent exactly as previewed.
- [x] Multi-execution targets only explicitly selected live terminal sessions.
- [x] Logging is off by default and never captures credential prompts supplied by
  the application.
- [x] Restoring a workspace starts no process or network connection automatically.

### M10: Settings, Accessibility, and UX Completion

Deliverables:

- [x] Add terminal font, size, line spacing, cursor, scrollback, and bell
  controls with live application and persisted defaults.
- [ ] Add terminal palette, copy, paste, and hyperlink policies.
- [x] Add connection timeout and keepalive settings across host-key probes,
  terminal, SFTP, and tunnel dials.
- [ ] Add reconnect, proxy, known-hosts, and agent settings.
- [x] Add transfer concurrency, collision, and partial-file settings with a
  live bounded limiter, backend-owned collision resolution, failed-partial
  retention, restart-safe resume metadata, and explicit cleanup.
- [x] Add notification enablement, categories, a long-transfer threshold,
  permission status, and test delivery.
- [x] Add reset-to-default behavior.
- [ ] Add per-profile override indicators.
- [ ] Complete screen-reader labels, keyboard traversal, visible focus,
  contrast checks, and reduced-motion behavior.
- [ ] Replace remaining transient status-label messaging with actionable
  dialogs, inline validation, and an activity/error history.
- [ ] Ensure compact desktop layouts and smaller windows remain usable without
  text overlap.

Tests and exit gate:

- [x] Settings round-trip, migration, validation, and reset behavior are tested.
- [ ] Add and test per-profile override precedence.
- [ ] Make every primary workflow keyboard reachable and record the native
  accessibility smoke checks.
- [ ] Add automated layout tests for minimum supported desktop sizes and
  high-DPI scaling.

### M11: Cross-Platform Packaging and Release Pipeline

Deliverables:

- [x] Add the embedded application icon, metadata, and default assets.
- [ ] Add the distributable dependency-license inventory.
- [ ] Promote the native CI jobs introduced in M0/M2 into the complete release
  matrix for macOS arm64/amd64, Linux amd64, and Windows amd64.
- [x] Embed version, commit, build date, and dirty state in native development
  packages and expose them through the Settings workspace.
- [ ] Build final release-mode binaries with stripped debug symbols and validate
  their supplied release metadata.
- [x] Build and launch a local macOS arm64 development `.app` with embedded
  frontend assets.
- [ ] Package release builds as macOS `.app`, Windows GUI `.exe`, and Linux
  executable/package forms.
- [ ] Add code-signing and notarization hooks for the final release; unsigned
  local-development builds remain supported.
- [ ] Generate checksums, an SBOM, a dependency-license report, and release
  notes.
- [ ] Verify clean-machine startup and expected config, cache, log, and keychain
  paths on every OS.
- [ ] Verify WebView prerequisites and failure messages on supported Windows and
  Linux versions.
- [x] Document explicitly that `make run` is a foreground developer command
  while packaged apps launch through the desktop environment.

Tests and exit gate:

- [ ] Each release artifact launches on a clean supported OS image.
- [ ] Verify that Windows opens no console window.
- [ ] Verify that macOS launches from Finder and passes signing/notarization when
  credentials are configured.
- [x] The audited macOS development package uses embedded frontend assets and
  starts no runtime HTTP listener or unpacked helper executable.
- [ ] Verify embedded frontend assets and migrations after relocation and
  without network access on every release target.

### M12: 1.0 Hardening

Deliverables:

- [x] Keep the current macOS Go unit, integration, race, vet, frontend unit, and
  production-build suites passing at the audited revision.
- [ ] Add and run the complete UI, fuzz, and native cross-platform suites.
- [ ] Add goroutine and file-descriptor leak checks around every long-lived
  service.
- [x] Run `govulncheck`, high-severity npm audit, and Go vet automatically in CI;
  the current local scans pass and the module-only advisory is recorded.
- [ ] Complete release-time dependency review, broader static analysis, and a
  manual threat-model review.
- [ ] Profile startup and idle CPU behavior.
- [x] Packaged-macOS terminal flood and bounded scrollback memory behavior under
  the M1 10 MiB workload.
- [x] Packaged-macOS 15-minute, eight-session terminal soak with sustained input,
  visibility rotation, exact queue drain, deterministic close, and bounded
  steady-state RSS growth.
- [ ] Large directory listing, large transfer, multi-hour terminal soak, and
  broader many-session behavior.
- [ ] Add structured local diagnostics and a user-controlled diagnostic export
  with redaction.
- [ ] Complete user documentation for profiles, trust prompts, credentials,
  transfers, tunnels, settings, data locations, and recovery.
- [ ] Remove every placeholder, fake-success state, debug action, and dead code
  path.

Release gate:

- [ ] Prove that idle with no sessions performs no polling and has no sustained
  background work.
- [ ] Prove that all resources close deterministically on tab close and
  application exit.
- [ ] Make core workflows pass on macOS, Linux, and Windows.
- [ ] Confirm that no known critical or high-severity vulnerability remains
  without a documented mitigation and release decision.
- [ ] Prove that configuration can be migrated and recovered without losing
  secrets or profiles.
- [ ] Verify a complete packaged SSH, SFTP, and tunnel workflow without
  launching it from a terminal or installing an application helper process.

## 7. Test Strategy

### 7.1 Unit Tests

- Domain validation and state transitions.
- Profile and settings precedence.
- Config migrations and atomic persistence.
- Go/TypeScript bridge DTO compatibility, frontend lease/session generation
  validation, event sequencing, cumulative acknowledgement, and bounded
  round-robin output scheduling.
- Frontend terminal-controller output queueing, ordered `onData`/`onBinary`
  input, resize coalescing, malformed-frame validation, parser failure
  containment, StrictMode remounting, and idempotent disposal.
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

| Milestone | Current status | Expected effort | Release value |
| --- | --- | ---: | --- |
| M0 Foundation | Complete | 4-7 days | Wails migration and honest lifecycle |
| M1 Terminal proof | Partial; performance and native stress gates open | 7-12 days | Proven PTY/bridge/xterm vertical slice |
| M2 Local terminal | macOS core implemented; Windows/Linux gates open | 8-14 days | First genuinely usable program |
| M3 Workspace | Partial; tab-management and Activity work open | 4-7 days | Reliable multi-session desktop UX |
| M4 Profiles/config | Mostly implemented; recovery and overrides open | 4-7 days | Durable daily workflow |
| M5 Credentials/trust | Partial; OS secret storage remains open | 5-8 days | Secure SSH foundation |
| M6 SSH | Core implemented; advanced connection modes open | 7-12 days | Primary remote workflow |
| M7 SFTP | Core implemented; local pane and stress gates open | 8-14 days | File workflow |
| M8 Tunnels | Implemented; two integration fixtures open | 6-10 days | Network workflow |
| M9 Productivity | Implemented | 6-10 days | Operator efficiency |
| M10 UX/settings | Partial | 5-9 days | Product completeness |
| M11 Packaging | macOS arm64 development bundle with identity only | 5-10 days | Installable native releases |
| M12 Hardening | In progress; release gates open | 7-14 days | 1.0 release confidence |

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
