# Remote Projects and Self-Hosted Editor Plan

Status: proposal, not implemented. Last reviewed: 2026-07-17.

This document proposes an end-to-end Remote Projects feature. A user adds an
SSH host, connects with the existing trust and authentication flow, selects a
remote directory, installs a self-hosted Code OSS editor on that host when
needed, saves the directory as a local project, and opens it from `shh-h` in
the local system browser. No desktop VS Code installation is required on the
client and no Microsoft tunnel or Microsoft-hosted editor page is used.

The editor frontend necessarily executes in the local browser, but its files,
extension host, terminal, processes, and project data remain on the SSH host.
All editor HTTP and WebSocket traffic reaches the host through the existing
SSH trust boundary.

This is an optional post-1.0 track. It is not a small extension of saved
workspace layouts or generic user-configured tunnels.

The initial project transport uses an ordinary SSH profile. A future
Teleport-backed project is gated on Teleport-specific remote execution,
transfer, forwarding, reauthentication, and cleanup proofs; it must never fall
back to a node's direct address. See `docs/TELEPORT_INTEGRATION_PLAN.md`.

## 1. User Outcome

The complete happy path is:

1. The user creates or selects an SSH profile.
2. `shh-h` verifies the host key and authenticates exactly as it does for a
   terminal or SFTP session.
3. The user chooses New Project and selects a directory through SFTP.
4. `shh-h` inspects the remote operating system, architecture, prerequisites,
   and existing compatible editor installations.
5. If the managed editor is absent, the app shows the exact provider, version,
   source, license, expected size, install location, and commands before asking
   for consent.
6. The app installs the editor into the remote user's data directory without
   `sudo`, a package-manager change, a service, or shell-startup modification.
7. The app saves a local project record containing the SSH profile ID and
   canonical remote path. It saves no SSH credential, editor token, port, PID,
   or OAuth URL.
8. Open Project explicitly connects SSH, starts the remote editor on remote
   loopback, starts an ephemeral local gateway, and opens the project in the
   operating system's default browser.
9. A CLI running in the managed editor terminal can request that `shh-h` open
   an authentication URL locally. A loopback OAuth callback is forwarded back
   to the remote CLI when it can be identified safely.
10. Closing the project or app stops the gateway, callback forwards, remote
    editor process, browser-request bridge, and SSH leases. The saved project
    remains disconnected and can be opened again later.

The project sidebar shows disconnected, connecting, provisioning, starting,
ready, stopping, and failed states. Restoring the application never connects,
installs, updates, or starts an editor automatically.

## 2. Product and Compliance Decisions

### 2.1 Default Provider

The automated provider should be
[OpenVSCode Server](https://github.com/gitpod-io/openvscode-server), referred
to in product text as "OpenVSCode Server" or "Code editor," not as the
Microsoft Visual Studio Code product. Its repository currently publishes the
Code OSS-based server under the
[MIT license](https://github.com/gitpod-io/openvscode-server/blob/main/LICENSE.txt)
and documents a connection-token mode. The release process must recheck the
license, notices, release provenance, supported platforms, and authentication
behavior for every pinned update.

The provider boundary must remain narrow enough to add `code-server` or an
organization-managed Code OSS build later. The first release supports one
provider well; it does not present a vague executable field that can run an
arbitrary remote command.

The branded Microsoft VS Code Server is not the default automated provider.
Its current license limits how the server may be used and expressly restricts
hosting, sharing, combining it with another application for others to use, and
stand-alone offerings. Microsoft also documents it as a single-user product
and says it may not be hosted as a service. See the
[VS Code Server license](https://code.visualstudio.com/license/server) and
[VS Code Server documentation](https://code.visualstudio.com/docs/remote/vscode-server).
Launching an independently installed `code serve-web` can be considered only
as a separately labeled bring-your-own provider after product-specific legal
review. `shh-h` must not auto-install it, silently accept its license, bundle
it, rebrand it, or claim that the Code OSS license covers Microsoft's binary.

This is an engineering compliance gate, not legal advice. A distributable
release still requires a license and trademark review for the chosen provider,
its dependencies, its extension registry, and the way `shh-h` provisions it.

### 2.2 No Hosted Control Plane

The normal project path must not call `code tunnel`, `vscode.dev`, or another
relay service. The data path is always:

```text
local system browser
        |
        | HTTP/WebSocket on an ephemeral 127.0.0.1 port
        v
shh-h loopback gateway (memory-only authentication)
        |
        | SSH direct-tcpip channel
        v
remote 127.0.0.1:<ephemeral editor port>
        |
        v
OpenVSCode Server and the selected remote project
```

The editor may make outbound requests for updates, extensions, source-control
providers, or extensions' own services unless policy disables them. Those are
separate, visible egress choices. The managed defaults disable editor
telemetry, experiments, and self-update where the selected provider supports
that configuration. `shh-h` manages editor updates. Extension discovery uses
Open VSX or an administrator-configured registry, never an undocumented use of
Microsoft's marketplace. Offline mode disables registry access entirely.

The workbench, service worker, extension-webview bootstrap, terminal, and
editor transport must remain functional without Microsoft-hosted runtime
assets. RP0 records browser and remote-host network traffic during a complete
workflow. A provider build that silently depends on `vscode.dev`, a Microsoft
CDN for executable editor content, or a Microsoft relay is rejected or rebuilt
with reviewed self-hosted assets before release. User-selected extensions and
source-control services may have their own separately disclosed endpoints.

### 2.3 Installation Consent and Ownership

- Installation is a separate, explicit action after SSH authentication. A
  Connect action does not imply consent to install software.
- Install into a resolved user-owned XDG data directory, such as
  `$XDG_DATA_HOME/shh-h/editors/openvscode/<version>`, with a documented
  fallback under the remote user's home directory.
- Never request `sudo`, edit `/usr`, add an apt/rpm repository, install a
  system package, alter a firewall, create a system service, or edit shell rc
  files.
- Do not install when the SSH account is root unless a later policy explicitly
  supports and warns about it.
- Show upstream source, version, platform artifact, SHA-256 digest, license
  link, disk estimate, and destination before confirmation.
- Record only non-secret install metadata and an acceptance/audit timestamp.
  Do not treat a click in `shh-h` as acceptance of a third-party EULA that the
  app is not authorized to accept.
- Deleting a local project never removes remote source files or editor data.
  Uninstall is a separate destructive workflow listing the exact managed
  directories. It cannot target an unverified path or follow a symlink outside
  the managed editor root.

### 2.4 Initial Support Matrix

The first implementation supports remote Linux x86-64 and arm64 hosts whose
libc and other runtime prerequisites match a reviewed provider release. Local
macOS is the first native client gate, followed by local Windows and Linux.
Remote macOS and Windows need their own path, process, helper, and cleanup
adapters and are not implied by a successful Linux implementation.

Unsupported hosts remain usable for normal SSH, SFTP, and tunnels. Inspection
must explain the missing prerequisite without attempting a partial install.

## 3. Persistent and Runtime Models

### 3.1 Local Project

A versioned, atomically written, private local store contains records
equivalent to:

```text
RemoteProject
  schemaVersion
  id
  name
  profileID
  remotePath             canonical absolute path
  editorProvider         openvscode
  installSelection       managed or administrator-provided
  createdAt
  updatedAt
```

`profileID` is the stable relation to the existing SSH profile. Renaming a
profile does not break a project. Deleting a profile referenced by a project
is blocked until the project is reassigned or removed.

The store never contains:

- Passwords, passphrases, private keys, agent material, or browser cookies.
- Editor connection tokens or browser-bootstrap capabilities.
- Local or remote ephemeral ports.
- Remote process IDs, SSH channels, or frontend lease IDs.
- Authentication URLs, OAuth state, authorization codes, or device codes.

### 3.2 Remote Managed Install

Each managed version contains a private manifest with provider, version,
platform, source URL, archive SHA-256, binary-relative path, install time, and
the `shh-h` installer schema version. The install root is shared by projects
for the same remote user; user data and extensions live outside versioned
program directories so an update can roll back without deleting settings.

Do not reuse or modify `~/.vscode`, `~/.vscode-server`, or another product's
installation. An administrator-provided executable must be selected explicitly
by absolute path and is never updated or removed by `shh-h`.

### 3.3 Live Runtime

The Go backend owns one runtime per open project generation. It contains:

- An immutable project and SSH-profile snapshot.
- Frontend lease ID, runtime ID, generation, context, and cancellation.
- A reference-counted authenticated SSH connection lease.
- One remote editor process and a verified process identity.
- One remote editor port bound to remote loopback.
- One local loopback gateway and its actual bound address.
- A random editor connection token and one-time browser capability held only
  in memory.
- An optional remote browser-request listener and its in-memory bearer token.
- Zero or more short-lived OAuth callback forwards.
- Bounded, redacted startup diagnostics and an idempotent close path.

Browser tabs are not runtime owners. The backend lease is. A frontend reload or
lease expiry closes all project-owned resources under the same rules as
terminals and tunnels.

## 4. Architecture Changes

The feature follows the existing dependency direction and adds only capability
boundaries that correspond to real system resources:

```text
frontend/src/feature/projects/       project list, setup, status, actions
internal/domain/project/             saved project validation
internal/domain/editor/              install and runtime state machines
internal/usecase/project/            project persistence and profile relations
internal/usecase/editor/             inspect, install, start, stop, update
internal/port/remoteexec.go           bounded SSH exec and process contract
internal/port/editorprovider.go       provider inspection and launch contract
internal/port/browser.go              local external-browser launcher
internal/adapter/projectstore/        versioned private atomic store
internal/adapter/remoteexec/          SSH exec/process implementation
internal/adapter/openvscode/          reviewed provider implementation
internal/adapter/editorgateway/       loopback HTTP/WebSocket to SSH proxy
internal/adapter/browseropen/         native default-browser implementation
```

The existing SSH connection abstraction must first support reference-counted
connection groups. Editor exec sessions, SFTP provisioning, direct forwarding,
remote forwarding, and an optional terminal must be independent leases on one
authenticated connection. Closing one channel cannot tear down the others.

The remote exec port accepts a structured executable, argument, and environment
specification. SSH ultimately transmits a command string, so the Linux adapter
uses one audited POSIX argument encoder, rejects NUL/newline ambiguity, and
fuzz-tests every boundary. Provider code cannot concatenate project paths,
tokens, URLs, or usernames into shell source. Windows will require a separate
PowerShell-native encoder rather than reusing POSIX quoting.

The editor gateway is a deliberate exception to the current no-local-web-server
runtime promise. It exists only on loopback while a project is open, has no
Wails bindings, serves no `shh-h` frontend assets, and exits with its project
runtime. This change requires an ADR before implementation is promoted from a
spike.

Remote editor content is never loaded into the privileged Wails WebView. The
MVP always opens an external system browser. A future in-app editor would need
a separate unprivileged native webview process/window with no Go bindings,
isolated storage, an explicit navigation policy, and its own security review.

## 5. End-to-End Protocol

### 5.1 Connect and Inspect

1. Resolve the project profile and start the existing host-key probe. A new or
   changed host key follows the existing explicit decision flow; this feature
   has no bypass.
2. Authenticate using agent, key, keyboard-interactive, or session-only
   credentials. Clear password and passphrase buffers after the SSH connection
   is established. Password-backed runtimes do not reconnect unattended.
3. Use SFTP `RealPath` and metadata operations to canonicalize the selected
   directory and prove that it exists, is a directory, and is readable by the
   SSH user. Surface read-only status before saving.
4. Probe a fixed allowlist of facts: OS, architecture, libc compatibility,
   available disk, XDG/home paths, temporary-directory behavior, and the
   managed manifest. Use bounded output and timeouts.
5. Inspect only known provider locations and explicitly configured absolute
   paths. Do not execute the first `code` found through an attacker-controlled
   `PATH`.
6. Return a typed inspection result: compatible installed version, installable
   target, unsupported host, corrupt managed install, or policy-blocked host.

Inspection is read-only. It does not create directories, download an artifact,
start a listener, or accept a license.

### 5.2 Provision the Editor

1. Select an exact reviewed release from a manifest shipped in the signed
   `shh-h` build. Do not resolve `latest` at runtime.
2. Download over HTTPS, verify the final URL against an allowlist, enforce size
   and timeout limits, and compute SHA-256 while streaming. The default path
   may stream the archive through `shh-h` and SFTP; it is never installed or
   executed locally and any temporary local archive is private and deleted.
   A remote-direct download mode still verifies the same pinned digest.
3. Upload as a `0600` partial file under the managed remote cache. Never pipe
   network bytes directly into a shell or extractor.
4. Recompute the digest on the remote file, preferably through a bundled small
   verifier/helper or a probed system tool with strict output parsing. A
   mismatch deletes only the partial file and fails closed.
5. Reject archives with absolute paths, `..` traversal, special device entries,
   unsafe links, or unexpected top-level layout before extraction.
6. Extract into a new staging directory under the managed root. Reject symlink
   components in every parent path and set user-only ownership and permissions.
7. Run the managed binary by absolute path with `--version` and `--help`, under
   a timeout, to verify identity and required flags. Do not start the service
   yet.
8. Write the managed manifest atomically and rename staging to the final
   version directory. Keep the prior version for rollback.
9. Report success with the installed version and disk use. Installation does
   not start or open the project.

Updates repeat this process alongside the current version, pass a health check,
and switch atomically. A failed health check leaves the working version
selected. Cleanup of old versions is explicit and can remove only directories
named in valid managed manifests.

### 5.3 Start and Open a Project

1. Generate at least 256 bits of randomness for the remote editor connection
   token and a separate one-time local browser capability.
2. Start OpenVSCode Server by its verified absolute path with the selected
   folder, app-owned data directories, telemetry/update policy, remote host
   `127.0.0.1`, a provider-supported dynamic port, and connection-token
   authentication. Never pass `--without-connection-token`.
3. Keep the SSH exec channel attached. Capture only bounded startup diagnostics
   and redact token-shaped values before publishing errors.
4. Parse a strict provider readiness record, verify the reported port is a
   remote loopback port, and make a probe over an SSH direct channel. If the
   provider cannot bind port `0`, use bounded randomized retries and verify the
   listener rather than trusting a prior "free port" check.
5. Start the local gateway on `127.0.0.1:0`. It proxies HTTP and WebSocket
   traffic through SSH to the exact remote loopback endpoint. It does not honor
   proxy environment variables or arbitrary upstream destinations.
6. Open a one-use local bootstrap path in the system browser. The gateway
   consumes the capability, sets a host-only, `HttpOnly`, `SameSite=Strict`
   memory-session cookie, and redirects to a clean project URL. The remote
   editor token is added only on the upstream side and never appears in browser
   history, app configuration, events, diagnostics, or logs.
7. Require the exact local `Host` and same-origin `Origin` for HTTP and
   WebSocket upgrades. Reject CORS, DNS-rebinding hosts, missing session
   cookies, oversized headers, control paths after bootstrap, and all methods
   the provider does not need.
8. Show Ready only after the browser endpoint and upstream WebSocket path pass
   a health check. Keep the runtime visible in Activity even when the browser
   window is closed.

SSH already provides confidentiality and integrity between `shh-h` and the
remote loopback listener. TLS is not added inside that tunnel for the MVP. No
editor listener binds a LAN, VPN, container bridge, or public interface.

### 5.4 Stop and Recover

Stop is idempotent and ordered:

1. Reject new browser and callback requests.
2. Close the local gateway and every callback listener.
3. Close the remote browser-request listener.
4. Ask the remote editor process to terminate and wait for a bounded grace
   period.
5. Escalate to process-group termination only after verifying the recorded PID,
   start fingerprint, executable path, and runtime nonce. Never signal a PID
   based on a stale file alone.
6. Close SSH channels and release the connection lease.
7. Clear editor, bootstrap, helper, and URL material from in-memory owners.
8. Publish stopped or failed-with-cleanup-details and remove the live runtime.

The remote launcher uses a private runtime directory and a fixed supervisor
contract so a dropped SSH channel cannot silently orphan the editor. The next
connection inspects stale runtime records, verifies process identity, and
offers safe cleanup. It never kills an unknown process.

## 6. Browser and CLI Authentication Bridge

The browser bridge helps a CLI running in the managed remote editor terminal
open its authorization page on the user's local machine. It transports the
flow; it is not an OAuth client and never exchanges, stores, or interprets an
authorization code or access token.

### 6.1 Remote Open Helper

`shh-h` embeds a small, versioned, open-source helper for each supported remote
platform and uploads it into the managed editor directory. It is executed only
on the remote host. The editor process receives these runtime-only variables,
which its integrated terminals inherit:

```text
BROWSER=<absolute managed path>/shhh-remote-open
SHHH_BROWSER_BRIDGE=http://127.0.0.1:<remote-forwarded control port>
SHHH_BROWSER_TOKEN=<random per-runtime bearer value>
```

The helper accepts one URL, caps its size, and sends a versioned request to a
remote-loopback listener created with SSH remote forwarding. The listener's
accepted channels terminate directly in the Go backend; there is no local
network listener for this control protocol. The helper has no SSH credential,
browser cookie, Wails capability, editor token, or persistent configuration.

Do not modify `.bashrc`, `.zshrc`, system `xdg-open`, or the global remote
`PATH`. `BROWSER` covers compatible CLIs launched by the managed editor. Tools
that ignore it use their documented device-code or no-browser mode. Provider-
specific wrappers may be added only after compatibility and security tests,
never by silently shadowing common system executables.

Ordinary `shh-h` SSH terminals can receive this integration later through an
explicit shell-integration mode. OpenSSH servers commonly reject arbitrary
environment requests, so the implementation must not claim that sending an
SSH `env` request alone makes it reliable.

### 6.2 Local Open Decision

Every helper request is untrusted because any code running as the remote user
may invoke the helper. The app:

- Accepts only `https` authorization pages, plus literal IPv4/IPv6 loopback
  `http` URLs needed for local flows.
- Rejects URL user-info, control characters, invalid encodings, unsupported
  schemes such as `file`, `data`, and `javascript`, and oversized values.
- Displays the canonical Unicode and ASCII/punycode hostname, source project,
  and requested action in a native/app-controlled confirmation.
- Requires Open once by default. A remote terminal escape sequence alone can
  never open a local URL.
- Opens the approved URL with the operating system's default external browser,
  never the privileged Wails WebView.
- Holds the full URL only long enough to launch it. Logs and history record at
  most the request time, project ID, decision, and redacted hostname.

Using the external browser follows the native-app OAuth guidance in
[RFC 8252](https://www.rfc-editor.org/rfc/rfc8252.html). Embedded webviews are
not used for sign-in.

### 6.3 Loopback Callback Relay

Many CLIs start a listener on the remote loopback interface and either ask the
browser to open that loopback URL directly or include it as a `redirect_uri`
in an external authorization URL. A browser on the client would otherwise send
either request to the client's loopback interface.

For a safely recognizable callback, `shh-h`:

1. Parses but does not rewrite the requested URL and any directly exposed
   `redirect_uri` value.
2. Accepts a callback target only when it is an HTTP URL with a literal
   `127.0.0.1` or `[::1]` host, an explicit unprivileged port, and a bounded
   path. `localhost` is supported only as a compatibility fallback with a
   visible warning because literal loopback addresses are less ambiguous.
3. Verifies over SSH that the exact remote loopback destination is listening.
4. Binds the same address family and port on local loopback. OAuth URLs are not
   modified to work around a local port collision.
5. Relays raw TCP bytes over SSH to that one remote address and port. It never
   parses the callback body, authorization code, or token.
6. Closes after a bounded flow timeout, an idle period after first use, project
   stop, frontend lease loss, or SSH disconnect.

If the callback is hidden in an opaque provider value, the local port is busy,
the CLI uses a custom URI scheme, or the target is not loopback, the app does
not guess or broaden forwarding. It opens only the approved authorization page
and shows an actionable fallback to the CLI's device-code or no-browser mode.
PKCE, OAuth state validation, redirect matching, and token exchange remain the
CLI and authorization server's responsibility.

The implementation begins with a compatibility matrix for representative
tools such as GitHub CLI, cloud CLIs, and extension-provided CLIs. A tool is
listed as supported only after its current documented flow passes an end-to-end
test; `BROWSER` support is not assumed from anecdotal behavior.

## 7. Security and Privacy Requirements

| Threat or asset | Required control |
| --- | --- |
| SSH host impersonation | Reuse strict known-host verification and changed-key hard failure. |
| Remote installer supply chain | Exact reviewed version, allowlisted HTTPS source, size cap, pinned SHA-256, safe archive extraction, atomic staging, and rollback. |
| Accidental privilege or host mutation | User-owned install only; no sudo, package repository, service, firewall, or shell-rc changes. |
| Public editor exposure | Remote and local loopback only, SSH transport, per-runtime provider token, authenticated local bootstrap, and no public bind option. |
| Remote content reaching app privileges | External browser only; remote content never shares the Wails origin or Go bindings. |
| Localhost cross-site or DNS-rebinding request | Exact Host/Origin checks, no CORS, one-time bootstrap, SameSite session cookie, destination pinning, and short lifetime. |
| Malicious remote URL or phishing request | Scheme/URL validation, visible canonical host, per-request confirmation, no OSC-triggered open, and no URL logging. |
| OAuth callback hijack | Exact literal-loopback destination, same local port, narrow TTL, no rewrite, no non-loopback forwarding, and device-flow fallback. |
| Editor/extension code execution | Keep Workspace Trust enabled, start new paths restricted/untrusted, install no extensions automatically, and explain that extensions run with the remote SSH user's authority. |
| Secret disclosure | Keep SSH and editor tokens in scoped memory, redact process output, never persist auth URLs/codes, and never put secrets in React state or events. |
| Orphaned process or listener | Frontend lease ownership, verified process identity, bounded escalation, leak tests, and stale-runtime reconciliation. |
| Deletion outside managed root | Canonical-path and manifest checks, no symlink traversal, explicit uninstall confirmation, and no wildcard deletion. |

Workspace Trust is not a sandbox and extensions can read files, run processes,
and make network requests with the remote user's permissions. Keep Restricted
Mode enabled for newly added projects and never auto-trust a directory. See
[Workspace Trust](https://code.visualstudio.com/docs/editing/workspaces/workspace-trust)
and
[Extension runtime security](https://code.visualstudio.com/docs/configure/extensions/extension-runtime-security).

The privacy inventory must explicitly cover SSH endpoint metadata, provider and
version metadata, project paths, browser-request hostnames, and update checks.
Terminal/editor contents, full authentication URLs, query strings, fragments,
device codes, callback bodies, access tokens, and extension data are excluded
from telemetry and diagnostic exports. The feature is useful with application
telemetry entirely disabled.

## 8. Bridge Surface and UI

The typed desktop bridge should expose task-level operations equivalent to:

```text
ListRemoteProjects() -> []RemoteProjectDTO
InspectRemoteProjectTarget(frontendLeaseID, profileID, remotePath, credentials)
PlanRemoteEditorInstall(frontendLeaseID, inspectionID) -> InstallPlanDTO
InstallRemoteEditor(frontendLeaseID, inspectionID, consentNonce)
CreateRemoteProject(input) -> RemoteProjectDTO
UpdateRemoteProject(input) -> RemoteProjectDTO
DeleteRemoteProject(projectID)
OpenRemoteProject(frontendLeaseID, projectID, credentials) -> EditorRuntimeDTO
StopRemoteProject(frontendLeaseID, runtimeID, generation)
ListRemoteProjectStates(frontendLeaseID) -> []EditorRuntimeDTO
ApproveBrowserRequest(frontendLeaseID, requestID)
DenyBrowserRequest(frontendLeaseID, requestID)
PlanRemoteEditorUninstall(frontendLeaseID, profileID, installID)
UninstallRemoteEditor(frontendLeaseID, planID, consentNonce)
```

Inspection and install plans are short-lived backend objects tied to the
frontend lease. A frontend cannot alter a plan's URL, digest, destination, or
command and submit it back. Credentials use the existing isolated secret input
flow and are not added to project DTOs.

Events report editor lifecycle, bounded provisioning progress, browser-open
requests with a redacted display URL, and cleanup failures. They never carry
connection tokens, full authorization URLs, callback payloads, or raw process
output.

The sidebar and project view provide:

- New Project, Open, Stop, Retry, Edit, Remove, Inspect Installation, Update,
  and Uninstall commands with familiar icons and tooltips.
- A remote-directory picker backed by the existing SFTP capability.
- A provisioning review showing source, version, hash, license, destination,
  and disk use before Install.
- A first-open trust decision for the selected directory.
- Visible editor and callback tunnel activity, actual local endpoint, elapsed
  time, and a clean Stop action.
- Actionable failures for unsupported hosts, hash mismatch, permission denied,
  disk full, port collision, process exit, SSH loss, callback incompatibility,
  and browser launch failure.

Removing a project is always local-only. Update and uninstall are unavailable
while any runtime uses the selected managed version.

## 9. Delivery Plan

### RP0: Provider, License, and Protocol Spike

- Record the provider decision and local gateway exception in ADRs.
- Review OpenVSCode Server license/notices, release artifacts, extension
  registry, telemetry/update behavior, supported platforms, token protocol,
  WebSocket behavior, workspace trust, and clean shutdown.
- Record browser and remote-host network traffic and prove that workbench,
  service-worker, extension-webview, and terminal runtime assets do not depend
  on a Microsoft-hosted page, CDN, or relay.
- Prove an external browser through a loopback gateway and SSH direct channel.
- Prove that the upstream token can remain server-side and does not appear in
  browser history, redirects, logs, or frontend events.
- Build a fake editor server for deterministic gateway and lifecycle tests.

Exit gate: legal/product review accepts the provider and naming, and the
packaged macOS app can open a fake remote project without exposing a listener
off loopback or granting remote content Wails access. Otherwise stop or choose
another OSS provider.

### RP1: Projects, Shared SSH, and Inspection

- Add project domain/store, profile relation checks, bridge DTOs, sidebar, and
  SFTP directory selection.
- Implement reference-counted SSH connection groups and bounded remote exec.
- Add typed host/provider inspection with no writes.
- Add disconnected restoration and complete error states.

Exit gate: projects round-trip across restart without connecting; host-key,
credential, path, timeout, cancellation, and corrupt-store tests pass.

### RP2: Secure User-Scope Provisioning

- Add the pinned release manifest, streaming downloader/uploader, digest
  verification, safe extraction, staging, managed manifest, rollback, update,
  and explicit uninstall.
- Generate an SBOM and third-party notices for app and remote helper artifacts.
- Add policy hooks to disable provisioning or require an administrator path.

Exit gate: a clean supported Linux fixture installs without sudo and a tampered,
truncated, oversized, traversing, or wrong-platform archive leaves no runnable
partial install. Uninstall cannot escape the managed root.

### RP3: Editor Runtime and Browser Gateway

- Add the runtime state machine, provider launcher, process supervision,
  dynamic remote port, local gateway, one-time browser bootstrap, health check,
  system-browser launch, Activity integration, and cleanup.
- Keep Workspace Trust enabled and start a new project in Restricted Mode.
- Add explicit update availability without background auto-update.

Exit gate: edit files, run an integrated terminal, reconnect explicitly, stop,
lose SSH, reload the frontend, and close the app without a leaked process,
socket, token, or goroutine. A scan from another machine finds no editor port.

### RP4: Browser Requests and OAuth Callback Relay

- Build and embed the remote open helper for supported platforms.
- Add the authenticated SSH remote-forward control listener, approval UI,
  strict URL parser, external browser launcher, callback detector, same-port
  local forwarder, TTLs, and device-flow fallback.
- Publish a tested CLI compatibility matrix.

Exit gate: a mock OAuth authorization-code flow with PKCE can start in a remote
CLI, open the local external browser after approval, return through the exact
loopback tunnel, and finish in the remote CLI. Malicious URL and callback
fixtures fail closed without leaking their values to logs or events.

### RP5: Hardening and Platform Promotion

- Run race, leak, long-session, reconnect, sleep/wake, proxy, offline, low-disk,
  and interrupted-update tests.
- Add local Windows and Linux native browser/gateway tests.
- Audit extension registry behavior and organization policy controls.
- Add remote macOS/Windows only with native process and path adapters.
- Complete accessibility, privacy, security, license, trademark, and release
  reviews.

Exit gate: every supported local/remote platform pair passes native packaged
tests and the complete acceptance criteria below.

## 10. Test Plan

Unit and fuzz tests cover:

- Project validation, migration, profile references, and private atomic storage.
- Provider manifests, platform selection, digest parsing, archive entry paths,
  symlink policy, and managed-root deletion guards.
- POSIX command argument encoding and rejection of NUL/newline ambiguity.
- Editor and install state transitions, stale generations, consent nonce
  replay, cancellation, timeout, and idempotent cleanup.
- URL parsing with user-info, Unicode/punycode, encoded controls, duplicate
  parameters, nested redirect values, fragments, unsupported schemes, huge
  inputs, and IPv4/IPv6 loopback edge cases.
- Gateway Host, Origin, cookie, bootstrap replay, WebSocket, upstream pinning,
  method, and header-size policies.
- Redaction proving that sentinel passwords, editor tokens, OAuth state, codes,
  full URLs, and query values do not enter logs, DTOs, events, or diagnostics.

Integration tests use an isolated SSH server and fixture artifacts to cover:

- Agent/key/password authentication, first-use trust, changed keys, disconnect,
  keepalive, and connection-lease independence.
- Clean install, reinstall, update, rollback, uninstall, read-only home, full
  disk, missing prerequisites, corrupt manifest, interrupted upload, and archive
  attacks.
- Real HTTP and WebSocket traffic through the gateway under port collision,
  browser refresh, concurrent project, and abrupt editor-exit conditions.
- Remote editor bound only to loopback and local gateway bound only to
  loopback, verified from separate network namespaces or hosts.
- Browser-helper authorization, replay, forged token, denied request, invalid
  URL, closed project, and helper-version mismatch.
- OAuth loopback success, local port collision, hidden callback, timeout,
  non-loopback redirect rejection, SSH loss, and device-code fallback.
- Process, channel, listener, file, goroutine, and browser-capability cleanup on
  every failure boundary.

Native UI tests cover the complete add-host -> add-project -> inspect -> install
-> open -> authenticate CLI -> stop -> reconnect -> remove flow. Browser-only
tests do not satisfy macOS, Windows, or Linux browser-launch and lifecycle gates.

## 11. Acceptance Criteria

The feature is ready only when all of the following are true:

- A supported clean Linux SSH account with no editor can be added, inspected,
  provisioned without root, saved as a local project, and opened from `shh-h`.
- The client has no local VS Code installation and no provider executable is
  installed or executed locally.
- No Microsoft tunnel, `vscode.dev`, or public editor endpoint is required.
  Document every remaining install, registry, source-control, and extension
  egress endpoint.
- The editor and app gateway never listen outside loopback, use independent
  runtime credentials, and disappear after Stop, lease loss, or app exit.
- Remote editor content cannot call Wails bindings or navigate the privileged
  app WebView.
- A representative browser-based CLI login succeeds end to end without the app
  storing or logging the authorization URL, code, callback body, or resulting
  token.
- Unsupported or opaque auth flows fail with a documented device-code or
  no-browser fallback and never with broadened forwarding.
- New project folders begin untrusted, no extension is auto-installed, and the
  user is told that trusted extensions execute with their remote SSH account's
  permissions.
- Provisioning is reproducible from an exact release manifest, rejects tampered
  artifacts, preserves rollback, and supplies required third-party notices.
- Removing a project is local-only; update and uninstall are explicit; no
  cleanup operation can remove source code or an unmanaged editor.
- Race, leak, security, packaged native, license, privacy, and accessibility
  gates pass for every advertised platform pair.

## 12. Explicit Non-Goals

- Installing desktop VS Code on the connecting computer.
- Using Microsoft Remote Tunnels, `vscode.dev`, Codespaces, or a `code tunnel`
  control plane.
- Offering a public or multi-user hosted editor service.
- Running the editor as root or a system daemon.
- Automatically trusting a workspace or installing recommended extensions.
- Capturing OAuth credentials, becoming an OAuth proxy, or emulating provider
  login pages.
- Transparently supporting every CLI that may launch a browser.
- Loading remote editor code in the privileged Wails WebView.
- Treating the Microsoft Visual Studio Code and Code OSS licenses as
  interchangeable.
