# ADR 0001: Desktop Frontend Stack

- Status: Accepted
- Date: 2026-07-17

## Context

The first scaffold used Fyne because it creates cross-platform Go desktop
applications and can produce one executable. The product, however, is centered
on terminal behavior. Staying with Fyne would require us to build and maintain
a custom terminal renderer, keyboard encoder, selection model, Unicode and IME
handling, scrollback, clipboard integration, accessibility behavior, and
high-throughput UI update path.

Those are mature terminal-emulator concerns, not useful product
differentiators. The candidate Go terminal engine was also untagged, leaving
both the emulator and renderer as high-risk foundations.

The desktop still needs a Go backend, embedded frontend assets, no helper
daemon, rich operational UI, and macOS, Windows, and Linux support.

## Decision

Use:

- A pinned stable Wails v2 release as the desktop host.
- React with strict TypeScript for application workflows and state.
- Vite for the production frontend build.
- A pinned stable `@xterm/xterm` release for terminal emulation and rendering.
- Stable xterm.js fit and search addons initially.
- Optional xterm.js WebGL acceleration only with an automatic standard-renderer
  fallback.
- An exact Node 24 LTS and npm toolchain, a committed npm lockfile, and `npm ci`
  for reproducible builds.

Do not use Wails v3 while it remains alpha. Do not use xterm.js beta builds,
proposed APIs, or experimental addons in the 1.0 dependency baseline.

Go remains responsible for profiles, configuration, secrets, host-key trust,
processes, PTY and ConPTY, SSH, SFTP, transfers, tunnels, runtime state, and
shutdown. React stores serializable application state. A non-React controller
owns each xterm.js instance, and terminal bytes never pass through React
component state. React StrictMode remains enabled; mounting a component never
creates a process or connection.

## Bridge Rules

- Frontend commands are task-oriented and typed.
- Each DOM instance owns a frontend lease, and each runtime has a session ID and
  generation. Stale commands and events cannot affect a replacement runtime.
- Terminal output is framed without byte loss and carries lease, session,
  generation, monotonic sequence, byte count, and final-state metadata.
- Output is batched, bounded by a per-session byte-credit window, cumulatively
  acknowledged after xterm.js parsing, fairly scheduled, and backpressured
  instead of dropped.
- xterm `onData` and `onBinary` use one ordered, bounded input queue. Resize is
  debounced and coalesced while guaranteeing delivery of the final dimensions.
- Lifecycle and error events are separate from terminal output.
- Control and lifecycle traffic takes priority over bulk output.
- A frontend lease is renewed only while resources are active. Reload,
  replacement, or lease expiry closes owned runtimes after a bounded grace
  period; 1.0 does not implicitly reattach arbitrary shells.
- Production loads only embedded assets under a restrictive Content Security
  Policy.
- Remote navigation, unapproved bridge origins, production developer tools, and
  remote-triggered clipboard writes are disabled.

## Consequences

Benefits:

- Terminal compatibility, Unicode, IME, selection, mouse, clipboard,
  accessibility, and rendering build on a widely deployed terminal component.
- Complex profile, file, transfer, and tunnel workflows are easier to build and
  test with standard frontend tooling.
- Static frontend assets remain embedded in the Go executable.
- macOS uses the supported system WKWebView instead of making our renderer
  depend directly on deprecated OpenGL APIs.

Costs:

- The repository gains TypeScript, npm, Vite, and frontend dependency
  management.
- The Go/WebView bridge becomes a critical performance and correctness
  boundary.
- Manual WebView secret entry creates short-lived JavaScript data that cannot be
  reliably zeroed. Agent and keychain flows are preferred, and M5 must accept
  this threat-model limitation or require a native prompt.
- Windows depends on a supported WebView2 runtime and Linux on WebKitGTK.
- UI behavior must be smoke-tested in native WebViews, not only in a browser.
- Wails v2 may eventually require a deliberate migration after Wails v3 reaches
  a stable release.

## Validation Gate

M1 must prove a real macOS PTY through Go and Wails into xterm.js and back. The
gate covers ordered `onData` and `onBinary` input, resize, Ctrl+C, paste, IME,
Unicode, full-screen terminal programs, sequenced raw bytes, cumulative
acknowledgement, stale-generation rejection, frontend loss, fair scheduling, a
10 MiB output burst, explicit latency and queue budgets, inactive tabs, tab
close, window close, and process cleanup in a packaged arm64 `.app`.

If the gate fails, work pauses before SSH and broader UI development. The bridge
or host is reconsidered from measurements; the project does not default to
writing a terminal emulator from scratch.

## Alternatives Considered

### Fyne with a custom terminal

Keeps all source in Go but makes terminal rendering, input, accessibility, and
performance our responsibility. Rejected as the least stable route for a
terminal-centric application.

### Gio with a custom terminal

Offers lower-level rendering control but still requires most terminal and
desktop widgets to be built and maintained. Rejected because it does not remove
the main risk.

### Electron with xterm.js

Provides a proven terminal path but embeds a large browser runtime and weakens
the single-application and Go-first goals. Rejected.

### Tauri with xterm.js

Provides a smaller WebView host but moves the privileged backend and desktop
integration toward Rust. Rejected because Go is the chosen backend language.

## Distribution Meaning

"Single application" means one app-owned executable with embedded frontend
assets and no helper daemon, local web server, Node runtime, or sidecar program.
It does not mean statically embedding operating-system GUI runtimes. macOS ships
an `.app` bundle using WKWebView, Windows requires WebView2, and Linux requires a
documented WebKitGTK runtime.
