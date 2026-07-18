# Teleport Access Integration Plan

Status: proposal, not implemented. Last reviewed: 2026-07-17.

This document proposes an end-to-end integration with Teleport, the access
platform documented at goteleport.com. A user registers a Teleport Proxy,
authenticates through the cluster's supported browser, SSO, MFA, or local-auth
flow, discovers authorized SSH nodes and leaf clusters, saves useful targets,
and opens an interactive terminal from shh-h.

The first release integrates with a separately installed, administrator- or
user-provided <code>tsh</code> executable. <code>tsh</code> runs on the
computer running shh-h; it is not installed on each target node. The Teleport
Proxy, Auth Service, and agents on target infrastructure remain
administrator-managed prerequisites. shh-h does not deploy or configure a
Teleport cluster.

This is an optional post-1.0 access-provider track. It changes the current
self-contained-runtime promise because it depends on a local third-party
client executable. It therefore has its own legal, security, compatibility,
and release gates.

## 1. User Outcome

The complete happy path is:

1. The user chooses Add Teleport Cluster and enters the Teleport Proxy address.
2. The user selects an existing <code>tsh</code> executable or an
   administrator-managed path. shh-h inspects its identity and version without
   running a shell.
3. shh-h contacts the proxy over verified TLS, discovers its Teleport version,
   and checks client compatibility before saving the cluster.
4. The user chooses Log In. shh-h starts <code>tsh login</code> with a private,
   cluster-scoped <code>TELEPORT_HOME</code> and key-agent mutation disabled.
5. For a browser flow, the app opens the operating system's external browser.
   <code>tsh</code>, the browser, and the Teleport Proxy complete SSO, MFA, and
   the local callback. shh-h never receives the identity-provider password or
   token.
6. The app reads authenticated status through bounded structured
   <code>tsh</code> output and shows the active user, roles, allowed
   operating-system logins, root or leaf cluster, and certificate expiry.
7. The user browses the SSH nodes and leaf clusters currently authorized by
   Teleport policy. Search and label filters run over a fresh resource result.
8. The user selects an allowed operating-system login and chooses Connect.
9. shh-h starts <code>tsh ssh</code> under a real local pseudoterminal and
   attaches it to the existing terminal tab lifecycle. Teleport continues to
   enforce MFA, access policy, certificate validity, and session recording.
10. The user may save the cluster/node/login selection as a local target
    shortcut. Opening a saved target always re-resolves and re-authorizes it.
11. Logout terminates cluster-owned operations, invokes
    <code>tsh logout</code>, verifies that the app-owned profile is gone, and
    leaves no callback listener or child process running.

Restoring the app or a workspace never starts <code>tsh</code>, contacts a
proxy, renews a certificate, submits an access request, or connects to a node
automatically.

## 2. Product and Compliance Decisions

### 2.1 Teleport Is an Access Provider

Teleport is not represented as another value in the existing local/SSH
profile protocol field. An ordinary SSH profile describes one network target,
one host-key trust decision, and one credential strategy. A Teleport cluster
describes an access authority that can expose changing resources, leaf
clusters, allowed logins, short-lived certificates, access requests, MFA, and
recording policy.

The UI and domain model therefore use three distinct concepts:

- **Cluster**: a saved Teleport Proxy and local client configuration.
- **Resource**: a node discovered dynamically from that cluster.
- **Saved target**: a local shortcut that remembers a resource selector and
  preferred operating-system login but grants no access by itself.

The sidebar should place Teleport under an Access or Teleport workspace, not
mix dynamic nodes into the saved SSH profile list. A later unified search may
show both kinds with unambiguous provider badges and different connect paths.

### 2.2 Supported Client Boundary

The default design orchestrates the official <code>tsh</code> CLI through a
narrow, typed subprocess adapter. This keeps authentication, certificate
handling, per-session MFA, proxy routing, and protocol compatibility in
Teleport's supported client instead of partially reimplementing them.

The adapter is not a general command runner:

- The frontend chooses typed actions, never executable arguments.
- The backend selects every subcommand, flag, and environment variable.
- No invocation passes through a shell.
- The executable path is absolute, inspected, and revalidated before use.
- User-controlled proxy, cluster, login, and resource values occupy separate
  argument elements and are validated before process creation.
- Structured operations accept only documented JSON schemas and size limits.
- Interactive operations run in a supervised PTY owned by a frontend lease.

Importing Teleport's full native client into the Go process is not the default.
The repository's <code>/api</code> module does not provide the complete
interactive client, while the implementation used by <code>tsh</code> has
different licensing and a large protocol/security surface. A native
integration requires its own ADR, legal review, update strategy, and
equivalent conformance suite.

### 2.3 Distribution and License Gate

The default release must not bundle, download, redistribute, install, or
silently replace an official Teleport Community Edition <code>tsh</code>
binary. The user or administrator provides a client under terms applicable to
their use. The app records the selected path and invokes that installation as
an independent program.

This conservative default is required because Teleport's current repository
and binary licensing is not uniform:

- Teleport documents the repository <code>/api</code> module under Apache 2.0
  and the remaining source under AGPL, with separate enterprise components.
  See the
  [Teleport repository license overview](https://github.com/gravitational/teleport#license).
- Official Community Edition binaries from recent releases are governed by
  the
  [Teleport Community Edition license](https://github.com/gravitational/teleport/blob/master/build.assets/LICENSE-community),
  which includes organizational eligibility and product-use restrictions.
- The actual terms, version, notices, trademark rules, and intended
  distribution model must be rechecked before every release.

Three distribution modes may be considered, in order:

1. **Bring your own tsh** is the initial and default mode.
2. **Organization-managed tsh** may use a policy-defined path or deployment
   mechanism owned by the customer's administrator.
3. **Managed or bundled client** is unavailable unless a written Teleport
   redistribution/OEM agreement, or a deliberately compliant open-source
   distribution strategy, has been approved and documented.

Product text uses "Teleport" only to identify compatibility with the external
product. It must not imply endorsement, certification, or ownership. Release
review covers trademark usage as well as software licensing.

This section is an engineering compliance gate, not legal advice.

### 2.4 No Security Bypass Mode

The integration does not expose or internally use:

- <code>--insecure</code> or an equivalent TLS-verification bypass;
- <code>--skip-version-check</code> or an equivalent compatibility bypass;
- a switch that suppresses cluster-required MFA;
- a switch that disables or evades Teleport session recording;
- automatic access-request approval or role assumption;
- SSH host-key prompts that substitute local trust for Teleport CA trust; or
- arbitrary extra <code>tsh</code> flags entered by the user.

Self-hosted clusters need a proxy certificate trusted by the operating system
or organization. Teleport's documentation treats self-signed certificates and
insecure client mode as development-only. Production integration fails closed
and explains how an administrator can establish a trusted CA instead. See
Teleport's
[self-signed certificate guidance](https://goteleport.com/docs/installation/self-hosted/self-signed-certs/).

### 2.5 Initial Support Matrix

The first implementation supports:

- local macOS arm64, followed by local Windows x64 and Linux x64/arm64;
- one user-provided <code>tsh</code> executable per saved cluster;
- supported Teleport client/server major-version combinations;
- direct and leaf-cluster SSH node discovery;
- browser SSO, WebAuthn, TOTP, and local authentication as presented by
  <code>tsh</code>;
- interactive SSH terminals through <code>tsh ssh</code>;
- saved Teleport target shortcuts; and
- explicit login, refresh, reauthentication, and logout.

Access requests are the first follow-up slice. Application access, database
access, Kubernetes access, Windows desktop access, Machine Identity, file
browsing, port forwarding, and Remote Projects support are separate phases.
Each has a different lifecycle and must not be implied by SSH terminal support.

## 3. Trust and Data Flow

### 3.1 Components

The integration has four security principals:

~~~text
shh-h desktop process
    |
    | typed local subprocess control, private TELEPORT_HOME
    v
administrator/user-provided local tsh
    |
    | verified TLS and Teleport protocols
    v
Teleport Proxy and Auth Service
    |
    | Teleport-authenticated connection
    v
enrolled SSH node
~~~

shh-h trusts the selected <code>tsh</code> executable to handle Teleport
credentials and protocols. It does not trust command output to be small, valid,
or safe to render. It trusts the Teleport Proxy only after normal TLS
validation. It does not make a separate trust-on-first-use decision for each
node.

### 3.2 Browser Authentication Flow

The browser path is:

~~~text
user clicks Log In
        |
        v
shh-h starts local tsh login with a loopback-only callback
        |
        | emits a validated browser URL
        v
operating-system external browser <----> Teleport Proxy / identity provider
        |
        | redirect to tsh's short-lived local callback
        v
local tsh stores short-lived identity in app-owned TELEPORT_HOME
        |
        v
shh-h reads status, never the browser token
~~~

The Wails WebView is not used for authentication. Embedded WebViews are often
blocked by identity providers and would expose cookies and navigation to the
application's renderer. The app uses the operating system's external-browser
API only after validating the URL produced by the active login process.

<code>tsh</code> owns the callback listener and token exchange. shh-h does not
proxy, parse, log, or persist the OAuth, SAML, or OIDC assertion. Cancelling
login stops the child process and its callback listener.

### 3.3 Terminal Flow

The terminal data path is:

~~~text
xterm.js terminal
        |
        | existing ordered Wails terminal events
        v
Go session manager and PTY transport
        |
        | stdin/stdout/resize/signals
        v
local tsh ssh
        |
        | Teleport proxy routing, certificate auth, optional MFA
        v
authorized SSH node PTY
~~~

The existing terminal session manager remains responsible for frontend
leases, generation checks, bounded output, resize ordering, exit events, and
shutdown. The Teleport adapter owns only safe command construction, process
environment, and Teleport-specific error classification.

Teleport may record the remote PTY according to cluster policy. Local shh-h
session logging remains off by default and is independent of Teleport
recording. The terminal UI visibly states when the connection is governed by
Teleport policy and may be recorded.

## 4. Persistent and Runtime Models

### 4.1 Teleport Cluster

A versioned, private, atomically written cluster store contains records
equivalent to:

~~~text
TeleportCluster
  schemaVersion
  id
  name
  proxyAddress          canonical host:port
  tshExecutablePath     absolute local path
  tshExecutableDigest   last acknowledged SHA-256
  tshVersion            last inspected version, informational
  authConnector         optional preferred connector name
  usernameHint          optional Teleport username hint
  group
  tags
  favorite
  createdAt
  updatedAt
~~~

The canonical proxy address contains no URL user information, path, query, or
fragment. The default port is the documented HTTPS proxy port. Input is parsed
with structured URL and host/port APIs, not string concatenation. The UI shows
the canonical ASCII hostname and port before first contact.

The executable digest is change detection, not proof that Teleport published
the file. When the executable identity or version changes, the app reinspects
it and asks the user to acknowledge the new client before login or connect.
Enterprise policy may replace that prompt with an administrator-pinned digest
or code-signing requirement.

The cluster store never contains passwords, MFA responses, private keys,
session certificates, browser URLs, callback ports, access-request secrets,
or identity-provider tokens.

### 4.2 Saved Target

Saved shortcuts are stored separately or in a provider-neutral target store:

~~~text
TeleportTarget
  schemaVersion
  id
  name
  clusterID
  leafClusterName       empty means root cluster
  resourceKind          ssh_node in the first release
  resourceID            stable Teleport resource ID when available
  resourceName          last known hostname/name fallback
  operatingSystemLogin  preferred login, not a credential
  labelSnapshot         display-only last known labels
  group
  tags
  favorite
  createdAt
  updatedAt
~~~

A target is not an authorization cache. Connect refreshes cluster status,
resolves the resource, verifies that the login is still allowed, and lets the
proxy make the final authorization decision. A missing, renamed, or no-longer
authorized resource produces a disconnected error rather than connecting to a
different node with a coincidentally similar label.

Deleting a cluster is blocked while targets reference it, unless the user
chooses a single reviewed action that deletes those local shortcuts too.
Neither action changes the Teleport cluster or any remote resource.

### 4.3 Credential Home

Each cluster receives an app-owned directory equivalent to:

~~~text
<private app data>/teleport/<cluster-id>/home
~~~

Every invocation for that cluster sets <code>TELEPORT_HOME</code> to this path.
The app does not use or modify the user's default <code>~/.tsh</code>
directory. Directories use owner-only permissions and files use the strictest
permissions compatible with <code>tsh</code>. The credential tree is excluded
from profile export, diagnostics, crash attachments, and cloud-sync features.
Platform no-backup/file-protection attributes are applied where available.

Teleport identities must remain readable by <code>tsh</code>, so the plan does
not claim application-level encryption that would be ineffective while the
process is running. Protection relies on the operating-system account, private
file permissions, full-disk encryption, short certificate lifetime, and
explicit logout. A future OS-keystore design requires compatibility testing
with Teleport's identity format before making stronger claims.

Forget Cluster may delete only a path derived from a validated app-owned
cluster ID beneath the known Teleport data root. It first terminates
cluster operations, performs logout where possible, rejects symlinks and
unexpected path ownership, and uses the same guarded-delete rules as other
sensitive app data.

### 4.4 Runtime State

Authentication state is derived, not persisted:

~~~text
TeleportAuthState
  clusterID
  phase                 disconnected, inspecting, login_required,
                        authenticating, authenticated, expiring,
                        expired, logging_out, failed
  teleportUsername
  activeClusterName
  roles
  allowedLogins
  certificateExpiresAt
  accessRequests
  generation
  lastError
~~~

The runtime never treats a stale authenticated value as authority. Status is
refreshed before resource listing, access-request changes, and new terminals.
Expiry timers are presentation aids; <code>tsh</code> and the proxy remain
authoritative.

Long-running login, terminal, forwarding, and request-watch operations have a
runtime ID, cluster ID, generation, cancellation function, process group, and
frontend lease. Stale events are rejected using the same ownership rules as
existing terminal sessions.

## 5. tsh Adapter Contract

### 5.1 Executable Selection and Inspection

The Add Cluster flow never searches and executes an arbitrary first
<code>tsh</code> from <code>PATH</code>. It may show detected candidates, but
the user or administrator must select one. The backend then:

1. converts the selection to an absolute cleaned path;
2. resolves and records symlink targets without allowing an unreviewed target
   change after save;
3. verifies a regular executable file owned by the user, root, or an approved
   administrator and rejects group/world-writable files or parent directories;
4. collects file identity, permissions, size, modification time, and SHA-256;
5. runs only the fixed version command with a short timeout and bounded output;
6. parses a supported semantic Teleport version; and
7. optionally verifies platform code signing or package provenance when an
   enterprise policy defines an expected signer.

Inspection results clearly say that path and version checks do not establish
publisher authenticity unless an administrator has configured provenance
policy.

The path, resolved target, file identity, and digest are checked again before
every process start. A replacement pauses the action for reinspection instead
of executing changed bytes under an old approval.

### 5.2 Version Compatibility and Updates

Teleport recommends matching the client major version to the cluster. Its
documented compatibility window generally allows a client one major version
older than the server, but not a newer client. The implementation must use the
current official
[upgrade compatibility guidance](https://goteleport.com/docs/upgrading/overview/)
as its release-time source of truth.

The preflight obtains the proxy version through the documented HTTPS
<code>/v1/webapi/find</code> discovery endpoint and compares it with
<code>tsh version --client --format=json</code> where that structured command
is supported. The version policy is data-driven and covered by fixtures for
every supported major. Unsupported or malformed versions fail with an
actionable message showing both versions.

By default every managed invocation sets
<code>TELEPORT_TOOLS_VERSION=off</code> where supported. This prevents
<code>tsh</code> managed-updates logic from downloading another binary into
<code>TELEPORT_HOME/bin</code> and re-executing code that the app has not
inspected. See Teleport's
[client tools managed-updates documentation](https://goteleport.com/docs/upgrading/client-tools-managed-updates/).

The app does not update <code>tsh</code>. It tells the user which compatible
version their administrator should provide. Cluster-managed client updates may
be enabled only as an explicit enterprise policy after legal approval,
provenance checks, and a design for inspecting the downloaded executable
before execution.

### 5.3 Environment

The adapter builds an allowlisted environment appropriate to the platform.
Expected entries include:

- <code>TELEPORT_HOME=&lt;cluster-private-home&gt;</code>;
- <code>TELEPORT_ADD_KEYS_TO_AGENT=no</code> and the corresponding supported
  CLI flag;
- <code>TELEPORT_TOOLS_VERSION=off</code> where supported;
- terminal values such as <code>TERM=xterm-256color</code> and
  <code>COLORTERM=truecolor</code> for interactive sessions;
- a reviewed locale, temporary-directory path, and platform essentials; and
- <code>SSH_AUTH_SOCK</code> only when a supported authentication mode
  explicitly needs the existing user agent.

Unreviewed inherited <code>TELEPORT_*</code>, debug, identity-file,
config-file, CDN, and update variables are removed. Standard operating-system
proxy variables are used only through an explicit app or administrator policy;
they are not silently inherited. The app never adds Teleport identities to a
global SSH agent. If a cluster requires agent-backed credentials, that choice
is visible and the adapter still prevents adding new keys.

### 5.4 Typed Commands

The first adapter surface is conceptually:

~~~go
type TeleportClient interface {
    InspectClient(ctx context.Context, executable string) (ClientInfo, error)
    InspectProxy(ctx context.Context, proxy ProxyAddress) (ProxyInfo, error)
    Login(ctx context.Context, spec LoginSpec) (LoginRuntime, error)
    Status(ctx context.Context, clusterID string) (AuthStatus, error)
    ListClusters(ctx context.Context, clusterID string) ([]LeafCluster, error)
    ListSSHNodes(ctx context.Context, spec NodeQuery) ([]SSHNode, error)
    OpenSSH(ctx context.Context, spec TeleportSSHSpec) (port.TerminalTransport, error)
    Logout(ctx context.Context, clusterID string) error
}
~~~

Access requests extend a separate capability interface after the core path is
stable. File transfer and forwarding do not appear until their own protocol
spikes pass.

Each method maps to a fixed command template for an explicitly supported
<code>tsh</code> major. The code contains no generic
<code>Run(args []string)</code> method exposed above the adapter. Values that
begin with a dash, contain control characters, exceed field limits, or cannot
be represented unambiguously are rejected. The adapter uses an end-of-options
marker only where the pinned CLI documents support.

### 5.5 Structured Output

Noninteractive discovery uses documented JSON output where available,
including status, cluster, node, and access-request commands. The adapter:

- captures stdout and stderr separately;
- caps each stream and total process runtime;
- decodes one supported schema per Teleport major;
- rejects unknown top-level shapes, duplicate/conflicting identity fields,
  invalid UTF-8, excessive nesting, oversized labels, and trailing garbage;
- normalizes timestamps and identifiers in the adapter, not the frontend;
- treats stderr as diagnostics, never as JSON fallback unless that exact
  command/version is documented and tested; and
- redacts paths, browser URLs, certificates, tokens, and identity material
  before returning an error.

Schema adapters live next to recorded, reviewed fixtures. A new Teleport major
cannot be marked supported until all fixtures and real-cluster smoke tests
pass.

Interactive login and SSH streams are not parsed as general structured data.
Only narrowly defined login URL and terminal-exit signals are recognized.

### 5.6 Process Supervision

The current Unix PTY adapter already implements resize, signals, process-group
termination, wait, and close. Teleport work should extract or extend that
mechanism into an exact-command PTY adapter while preserving existing local
shell behavior. Teleport must not pass through local shell resolution or gain
the local profile's arbitrary environment feature.

Every child process:

- starts in its own process group or Windows Job Object;
- has no stdin unless the operation explicitly requires it;
- is associated with one runtime generation and cancellation context;
- has bounded graceful termination followed by forced termination;
- is waited exactly once;
- closes pipes, PTYs, callback listeners, and temporary files; and
- cannot survive frontend lease expiry or application shutdown.

## 6. End-to-End Workflows

### 6.1 Add and Inspect a Cluster

The Add Cluster form contains a name, proxy address, <code>tsh</code>
executable selector, and optional username/connector hints. Save performs no
login.

Inspection proceeds in this order:

1. Validate and canonicalize local input.
2. Inspect the executable without a shell.
3. Connect to the proxy discovery endpoint over normal TLS with system and
   administrator-installed roots.
4. Permit only bounded HTTPS redirects that preserve the expected proxy host;
   reject downgrade, credentials in URLs, and unexpected origins.
5. Parse the proxy's public version/capability response with strict limits.
6. Check the supported client/server version matrix.
7. Show the canonical proxy, certificate identity, proxy version, client path,
   client version, digest, and any compatibility warning.
8. Persist only after explicit confirmation.

Private, loopback, and internal proxy addresses are valid for on-premises
clusters and are not categorically blocked. The UI makes the destination clear
and never follows a proxy-supplied instruction to run a local command.

### 6.2 Login and External Browser Authentication

Login creates a dedicated visible auth runtime. The preferred flow uses the
supported equivalent of <code>tsh login --browser=none</code> so the app can
receive the login URL and open it through the operating system. It also pins
the callback bind address to literal loopback where the supported client
provides that control.

The URL recognizer accepts only a single URL emitted by the current login
runtime and only one of these reviewed forms:

- HTTPS to the configured Teleport Proxy origin; or
- HTTP/HTTPS to literal <code>127.0.0.1</code> or <code>[::1]</code>, with a
  valid ephemeral port, nonempty opaque path, no user information, and no
  suspicious fragment.

The exact accepted forms are version-specific fixtures. Hostnames that merely
resolve to loopback are not accepted as callback URLs. The app opens the URL
only while the generating child is alive and after showing the expected proxy
identity. It never renders the URL as HTML or places it in logs, history,
telemetry, clipboard, or persisted state.

If a supported <code>tsh</code> version cannot provide a safely recognizable
URL, the fallback is an explicitly documented mode in which that local
<code>tsh</code> opens the system browser itself. The app must not scrape
arbitrary terminal text and open every URL it finds.

For password, TOTP, or other terminal prompts, login runs in a disposable auth
PTY with local session logging disabled. Input is sent directly to
<code>tsh</code>; the frontend does not retain it and the backend does not
attempt brittle prompt classification beyond display-safe redaction. SSO is
preferred where the cluster offers it.

WebAuthn and hardware-key user presence remain between the browser,
<code>tsh</code>, and the authenticator. The app does not simulate or suppress
those ceremonies.

Success is not inferred from a line of text. After <code>tsh login</code>
exits, the app runs a fresh structured status command against the isolated home
and requires a valid unexpired identity for the expected proxy. Cancellation,
timeout, or a mismatched proxy returns to <code>login_required</code> and
cleans up the runtime.

### 6.3 Status, Expiry, and Reauthentication

The cluster view refreshes status on explicit open, before privileged actions,
and at a bounded interval only while that view or an owned connection is live.
It shows:

- Teleport username;
- root and active cluster;
- assigned roles and outstanding access requests;
- allowed operating-system logins;
- certificate expiry in local time and as a countdown; and
- clear expired, expiring, incompatible-client, and logged-out states.

The app warns before expiry but does not silently launch a browser or renew a
session. Reauthenticate is an explicit action. Existing SSH sessions follow
Teleport's server/client behavior at certificate expiry; shh-h does not
promise that a live authorized session will be killed or preserved. Every new
session performs a fresh status preflight.

### 6.4 Resource and Leaf-Cluster Discovery

After authentication, the app obtains leaf clusters with the documented
structured cluster command and SSH nodes with the documented structured node
list command. The root cluster is represented explicitly.

Resource results are ephemeral and bounded. The frontend receives normalized:

~~~text
TeleportSSHNode
  resourceID
  name
  hostname
  addressHint          display only, never connected directly
  leafClusterName
  labels
  description
~~~

Search supports name, hostname, cluster, and label key/value. Labels are
untrusted display data: they are escaped by React, length-limited, never used
as markup, and never converted into command arguments except through an
explicit documented resource selector.

Selecting a leaf cluster refreshes resources for that cluster. Results from an
older query generation cannot replace a newer selection. Pagination or server
limits are honored when the CLI exposes them; otherwise the adapter imposes a
safe maximum and tells the user the result is truncated.

### 6.5 Open an SSH Terminal

Connect requires an authenticated identity, a freshly resolved node, and an
allowed operating-system login. The adapter builds the version-specific
equivalent of:

~~~text
tsh ssh --proxy=<proxy> --cluster=<cluster> <login>@<node>
~~~

The exact flag order and selector syntax are verified against every supported
major; this example is not a user-editable command template.

The process starts under a PTY with the requested rows and columns. The session
manager registers it before output is forwarded and then applies the existing
lease, generation, backpressure, signal, and cleanup rules. The terminal title
contains the operating-system login, node, and Teleport cluster with control
characters removed.

Escape-sequence behavior from <code>tsh</code> is disabled where a documented
flag allows it, unless a later design deliberately exposes a nonconflicting
escape menu. This prevents Teleport's client escape handling from unexpectedly
competing with terminal shortcuts.

If cluster policy requires per-session MFA, <code>tsh</code> presents that flow
for the new connection. The app keeps the session visible and does not retry in
a way that could produce repeated prompts. See Teleport's
[per-session MFA documentation](https://goteleport.com/docs/zero-trust-access/authentication/per-session-mfa/).

### 6.6 Save and Open a Target

Save Target records the stable resource identity where available, last known
name, leaf cluster, and preferred login. It does not copy a node address into
an ordinary SSH profile.

Open Target performs:

1. client and proxy version preflight;
2. status and expiry refresh;
3. explicit login when needed;
4. leaf-cluster and node re-resolution;
5. allowed-login validation; and
6. normal Teleport terminal creation.

If multiple resources match a legacy name-only shortcut, the app asks the user
to choose and update the target. It never picks the first result.

### 6.7 Logout and Forget

Logout is cluster-scoped. It first asks for confirmation when terminals or
other operations for that cluster are active. On confirmation it closes those
operations, runs the fixed proxy-scoped logout command, and verifies through a
new status call that no identity for the cluster remains in its isolated home.

Failure to contact the proxy does not justify claiming a successful logout if
local credentials remain. The UI offers a separate guarded Remove Local
Identity action that deletes only verified app-owned identity data after all
processes stop.

Forget Cluster additionally removes the non-secret cluster record. It does not
delete targets silently, change server-side roles, revoke other devices,
remove nodes, or alter the user's normal <code>~/.tsh</code> profiles.

### 6.8 Access Requests

After basic terminals are stable, add typed support for Teleport's documented
access-request commands. The UI supports:

- searching requestable roles and resources;
- entering a required reason where policy expects one;
- choosing a bounded requested duration;
- submitting a role or resource request explicitly;
- showing pending, approved, denied, expired, and canceled states;
- assuming an approved request only after user confirmation; and
- dropping an assumed request or logging out.

Approvals are never automatic. Reviewer workflows are out of scope until a
separate authorization-focused design exists. Server policy remains
authoritative even when local validation succeeds. See Teleport's
[access-request documentation](https://goteleport.com/docs/connect-your-client/request-access/).

### 6.9 File Transfer and Port Forwarding

The first release does not route the current SFTP browser through Teleport.
The existing SFTP implementation owns a native SSH transport and host-key
workflow that are not equivalent to <code>tsh</code> proxying and Teleport
certificates. Remote shell commands are not an acceptable imitation of SFTP
semantics.

A later transfer spike may provide explicit one-shot copy through supported
<code>tsh scp</code> behavior. Rich SFTP browsing requires a supported,
testable transport such as a reviewed OpenSSH proxy integration or a legally
approved native client. It must preserve atomic transfers, cancellation, path
safety, progress, and policy/MFA behavior before claiming parity.

Port forwarding is also deferred until each supported <code>tsh</code> major is
verified for local, remote, and dynamic forwarding semantics. The first
candidate is a visible, supervised local forward bound to loopback by default.
Every forward has an owner, actual bound address, cluster/target label, expiry
behavior, and deterministic process cleanup. No hidden background
<code>tsh</code> process is allowed.

### 6.10 Relationship to Remote Projects

Remote Projects initially references an ordinary SSH profile. A later schema
may replace that single relation with a typed access target:

~~~text
ProjectAccessTarget
  provider              ssh or teleport
  profileID             for ssh
  teleportTargetID      for teleport
~~~

Teleport-backed projects require more than an interactive terminal. The app
must prove safe remote command execution, artifact upload, loopback forwarding,
process cleanup, reauthentication, and policy/recording disclosure through the
Teleport path. Remote editor provisioning must not fall back to a node's
direct address or bypass the Teleport Proxy. This integration is gated behind
the transfer and forwarding milestone and receives joint Remote
Projects/Teleport tests.

## 7. Application Architecture

### 7.1 Package Boundaries

The proposed packages are:

~~~text
internal/domain/teleport
    cluster.go
    target.go
    status.go
    validation.go

internal/port
    teleport.go

internal/usecase/teleport
    service.go
    auth_manager.go
    resource_service.go
    target_service.go
    access_request_service.go       later

internal/adapter/tshclient
    executable.go
    environment.go
    command_vN.go
    json_vN.go
    login.go
    terminal.go
    errors.go

internal/adapter/teleportstore
    cluster_store.go
    target_store.go
    identity_home.go

frontend/src/feature/teleport
    api.ts
    types.ts
    store.ts
    TeleportWorkspace.tsx
    ClusterForm.tsx
    LoginPanel.tsx
    ResourceBrowser.tsx
    TargetList.tsx
~~~

The domain package contains no CLI or Wails types. The adapter has no frontend
knowledge. The use case layer owns state transitions and policy. The bridge
maps narrow DTOs and events.

### 7.2 Ports

Add explicit ports for:

- executable inspection and proxy discovery;
- login runtime creation and cancellation;
- structured status/resource queries;
- Teleport terminal creation;
- private cluster/target persistence; and
- guarded identity-home lifecycle.

<code>TeleportTerminalFactory</code> should return the existing
<code>port.TerminalTransport</code>, allowing the session manager to reuse
terminal ownership without teaching it CLI details. The current local-shell
factory is not passed Teleport profile fields or secrets.

### 7.3 Session Manager Integration

Add a typed <code>OpenTeleport</code> path to the session use case. It accepts
only a validated cluster ID, resolved resource selector, login, dimensions,
and frontend lease. The Teleport use case completes status and resource
preflight, then passes an immutable terminal spec to the factory.

The resulting session identifies its provider and target for titles, shutdown
summaries, workspace snapshots, and logs. A saved workspace restores a
disconnected Teleport tab and never initiates login or connection. Reconnect
reruns the full target workflow.

### 7.4 Bridge API

The Wails boundary should expose methods equivalent to:

~~~text
ListTeleportClusters()
InspectTeleportCluster(input)
CreateTeleportCluster(input)
UpdateTeleportCluster(id, input)
DeleteTeleportCluster(id)

GetTeleportStatus(clusterID)
StartTeleportLogin(leaseID, clusterID)
CancelTeleportLogin(leaseID, runtimeID)
LogoutTeleportCluster(clusterID)
ForgetTeleportIdentity(clusterID)

ListTeleportLeafClusters(clusterID)
ListTeleportSSHNodes(clusterID, leafCluster, query)
OpenTeleportTerminal(leaseID, clusterID, resourceID, login, columns, rows)

ListTeleportTargets()
CreateTeleportTarget(input)
UpdateTeleportTarget(id, input)
DeleteTeleportTarget(id)
~~~

Access-request methods are added only with that milestone. No method accepts
raw CLI arguments, environment values, browser URLs, identity files, or shell
commands.

Events contain runtime IDs and monotonically increasing generations:

~~~text
teleport:inspection
teleport:login-state
teleport:status
teleport:resources
teleport:error
~~~

Terminal bytes continue through the existing terminal event path. Events are
bounded and contain redacted, display-safe fields only.

### 7.5 Frontend Experience

The Teleport workspace is a dense operational view:

- a cluster list with authenticated, expiring, expired, incompatible, and
  disconnected status;
- Add, Edit, Log In, Reauthenticate, Log Out, Refresh, and Forget actions;
- a root/leaf cluster selector;
- searchable node table with labels and saved-target state;
- an allowed-login selector before Connect;
- certificate expiry and active role/access-request summary;
- a visible notice that Teleport policy and session recording may apply; and
- a focused authentication panel while browser or terminal MFA is pending.

Buttons use existing icon and confirmation conventions. Secret prompts are not
put into generic form state. Raw certificates, callback URLs, binary command
lines, and unredacted stderr are not shown in normal UI.

An empty cluster list shows Add Teleport Cluster, not a marketing page. An
empty resource result distinguishes no access, expired authentication, filter
miss, and an actual cluster error.

## 8. Security and Privacy

### 8.1 Threat Model

| Threat | Required mitigation |
| --- | --- |
| User-controlled value becomes a flag or shell command | Never invoke a shell; fixed command templates; validate each argument; reject ambiguous leading-dash/control-character values. |
| A selected tsh is replaced with malicious code | Absolute path; file/parent permission checks; resolved target and identity checks; digest change acknowledgment; optional signer policy. |
| The app redistributes a client without valid rights | Bring-your-own client default; no downloader/updater/bundle; release license gate and artifact audit. |
| Proxy impersonation or TLS interception | System/admin trust roots; strict hostname validation; no insecure mode; bounded same-origin redirects. |
| Malicious CLI output opens a phishing page | Recognize one version-specific login URL only; enforce proxy or literal-loopback origin; external browser; no generic URL opening. |
| Callback listener is exposed to the LAN | Pin supported login callback to literal loopback; reject wildcard binds; terminate listener with auth runtime. |
| Credentials leak into shared Teleport state | Per-cluster app-owned TELEPORT_HOME; never use ~/.tsh; private permissions; no export/diagnostics. |
| Teleport keys are added to a global SSH agent | Disable key addition by flag and environment; test the agent before/after login; make required agent access explicit. |
| Client silently downloads and executes another binary | Disable managed-tools re-exec by default; reject unexpected executable changes; no app updater for tsh. |
| Oversized or malformed JSON exhausts resources | Stream and byte limits; deadlines; strict schema; nesting/string/count caps; kill and wait on violation. |
| Stale UI state grants access to a changed resource | Refresh status and re-resolve stable resource ID before connect; proxy remains authoritative; generation checks. |
| Certificate expires during work | Visible expiry; explicit reauthentication; fresh preflight for each operation; clear terminal/forward failure states. |
| Session content is recorded unexpectedly | Visible Teleport-policy disclosure before connect; link to organization policy; local logs remain independently opt-in. |
| A login or terminal child survives the app | Process groups/Job Objects; frontend leases; bounded shutdown; wait/reap tests; startup orphan audit. |
| Diagnostics reveal credentials or identity data | Structured redaction; no raw environment/profile archives; URL/token/certificate patterns treated as secrets. |
| Guarded cleanup deletes unrelated files | Derived app-owned paths only; root containment and ownership checks; reject symlinks; no user-provided delete path. |

### 8.2 Teleport Policy Remains Authoritative

The integration must preserve:

- short-lived certificate issuance and expiry;
- role-based access and resource filters;
- SSO and MFA required by cluster policy;
- per-session MFA challenges;
- access-request approval and assumption rules;
- proxy routing and node certificate validation; and
- cluster-side session recording and audit events.

shh-h does not claim that a successful local preflight guarantees access. The
Teleport Proxy may deny the operation and its reason is presented in a
redacted form. Teleport's
[session-recording architecture](https://goteleport.com/docs/reference/architecture/session-recording/)
is the source for administrator-side behavior; the app does not alter it.

### 8.3 Logging and Diagnostics

Normal logs may include operation type, cluster record ID, client/server major
versions, duration, exit classification, and redacted error category. They do
not include:

- full proxy URLs containing launch/callback data;
- certificate/key material or <code>TELEPORT_HOME</code> contents;
- passwords, TOTP codes, WebAuthn data, assertions, or tokens;
- raw <code>tsh status</code> identity output;
- terminal input/output unless the user separately enables session logging; or
- arbitrary node labels that may contain confidential metadata.

A support bundle includes only a manifest of redacted configuration and client
compatibility results after user preview. It never archives the Teleport home.

### 8.4 Session Recording Disclosure

Before the first Teleport terminal for a cluster, the app displays a concise
notice that the organization may record terminal sessions and audit commands
according to cluster policy. Acknowledgment is local UX state, not consent on
behalf of the organization and not proof that recording is enabled or
disabled.

The terminal header retains a Teleport/provider indicator so the trust context
is visible after connection. Local output logging cannot be turned on by a
Teleport cluster or inferred from server recording.

## 9. Failure Handling

Errors are classified into stable product categories:

- client missing, changed, untrusted, or incompatible;
- proxy address invalid, DNS/network unavailable, or TLS verification failed;
- login canceled, timed out, denied, or completed for a different proxy;
- MFA required, failed, or unsupported by the selected client/platform;
- identity absent, expiring, or expired;
- resource missing, ambiguous, or no longer authorized;
- operating-system login no longer allowed;
- access request pending, denied, expired, or canceled;
- structured output incompatible or exceeded safety limits;
- terminal process exited or was terminated; and
- local identity cleanup incomplete.

The frontend receives a category, safe summary, retryability, and optional
documented remediation. Raw stderr stays backend-only and is redacted before
debug use.

Partial operations roll back local state:

- failed Add Cluster writes no record or credential directory;
- canceled login kills the callback runtime and verifies no valid new identity;
- failed terminal startup removes the reserved session and PTY;
- failed target save leaves the previous atomic record intact;
- failed logout never reports local credentials as removed; and
- app shutdown closes all Teleport runtimes before stores are released.

The app never responds to failure by adding <code>--insecure</code>, skipping
version checks, falling back to direct SSH, or executing a different
<code>tsh</code> from <code>PATH</code>.

## 10. Implementation Milestones

### TP0: Legal, Protocol, and UX Proof

Deliverables:

- ADR for bring-your-own <code>tsh</code> and the optional-runtime dependency;
- recorded legal review of source, binary, trademark, and distribution terms;
- supported Teleport major-version matrix and release policy;
- proof of client/proxy version discovery over strict TLS;
- exact command/output fixtures for status, clusters, nodes, login, logout, and
  interactive SSH on each supported major;
- macOS proof that <code>--browser=none</code> or the selected fallback opens
  the external browser and leaves token exchange inside <code>tsh</code>;
- proof that callback bind is loopback-only and cancellation closes it;
- proof that isolated <code>TELEPORT_HOME</code> and no-agent-addition work;
- PTY proof for resize, signals, MFA prompts, exit status, and process cleanup;
- a session-recording/MFA/access-denial behavior report; and
- explicit findings for transfer, forwarding, and Remote Projects feasibility.

Exit gate: no production code beyond disposable spikes until license mode,
supported versions, browser URL forms, and process security are documented.

### TP1: Domain, Store, and Inspection

Deliverables:

- cluster and saved-target domain models with validation;
- private atomic stores, migrations, permission tests, and guarded identity
  homes;
- executable selector, inspection, digest change flow, and signer-policy hook;
- proxy address parser, TLS discovery, redirect policy, and compatibility
  service;
- typed bridge DTOs for list/inspect/create/update/delete; and
- cluster-list UI with disconnected and incompatible states.

Exit gate: adding or editing a cluster starts no login, creates no global
Teleport profile, and cannot execute an uninspected replacement binary.

### TP2: Login, Browser, Status, and Logout

Deliverables:

- auth runtime manager with leases, generations, timeout, and cancellation;
- isolated environment builder and managed-update suppression;
- external-browser URL recognizer and opener;
- disposable auth PTY for non-browser prompts;
- strict status parser, expiry presentation, and reauthentication state;
- logout, local-identity removal, and Forget Cluster workflows; and
- shutdown/process-leak and credential-permission tests.

Exit gate: SSO/MFA login and logout pass on a reviewed real cluster, browser
tokens never enter app state, no identity reaches <code>~/.tsh</code> or the
SSH agent, and cancellation leaves no listener or child.

### TP3: Resource Discovery and Interactive Terminal

Deliverables:

- root/leaf cluster and SSH node queries with schema fixtures;
- bounded searchable resource browser and allowed-login selector;
- exact-command PTY adapter and <code>TeleportTerminalFactory</code>;
- session-manager Open Teleport path and disconnected workspace snapshots;
- per-session MFA, certificate-expiry, denial, resize, backpressure, and
  shutdown tests; and
- persistent Teleport provider/recording indicator in the terminal UI.

Exit gate: a user can log in, discover an authorized node, connect, work in a
full PTY, close it, and exit the app with no orphan process or trust bypass.

### TP4: Saved Targets and Access Requests

Deliverables:

- target create/edit/delete and stale-resource re-resolution;
- provider-aware command-palette and workspace reconnect actions;
- requestable role/resource discovery and explicit request submission;
- pending/approved/denied/expired/canceled state handling;
- explicit assumption/drop behavior and expiry updates; and
- race tests for authorization changes between listing and connect.

Exit gate: shortcuts never become credentials, and access elevation is visible,
time-bounded, auditable through Teleport, and never automatic.

### TP5: Transfer, Forwarding, and Remote Projects Spikes

Deliverables:

- reviewed capability matrix for <code>tsh scp</code>, OpenSSH proxy
  integration, and supported forwarding modes;
- a decision to implement, defer, or reject each feature;
- loopback-only local-forward prototype with ownership and expiry behavior;
- transfer integrity/cancellation/path-safety proof if copy is accepted;
- Teleport-backed Remote Projects proof for remote commands, artifact install,
  editor loopback forwarding, reauthentication, and cleanup; and
- separate UX and security ADRs for every accepted capability.

Exit gate: no feature ships merely because a hand-written command worked once.
It must preserve current lifecycle and safety guarantees across supported
versions and platforms.

### TP6: Cross-Platform Hardening and Release

Deliverables:

- Windows Job Object, process, PTY, browser, path, and permission adapters;
- Linux process, browser, desktop-environment, and packaging coverage;
- real-cluster compatibility runs for all supported client/server pairs;
- signed-build testing with administrator-managed client paths;
- accessibility, localization, error-recovery, and large-resource testing;
- support-bundle redaction and privacy review;
- administrator deployment and CA-trust documentation; and
- final legal, security, and trademark sign-off.

Exit gate: all acceptance criteria pass on every advertised local platform and
no package contains an unapproved Teleport artifact.

## 11. Test Strategy

### 11.1 Unit Tests

Cover:

- proxy canonicalization, IPv4/IPv6, IDN display, ports, and rejected URL parts;
- executable path, symlink, ownership, mode, parent, digest, and replacement
  checks;
- client/server compatibility tables and malformed versions;
- environment allowlisting and removal of inherited Teleport/update variables;
- fixed argument generation for every command/version and injection payload;
- JSON schema, size, nesting, count, UTF-8, timestamp, and trailing-data limits;
- login URL origin, loopback literal, user-info, fragment, and control-character
  validation;
- cluster/auth/runtime state-machine transitions and stale generations;
- target resolution, ambiguity, changed roles, and allowed-login checks;
- redaction of browser links, identities, certificates, paths, and stderr; and
- guarded identity-home deletion and atomic store recovery.

### 11.2 Deterministic Fake-Client Tests

CI should build a repository-owned fake <code>tsh</code> fixture that
implements only the test protocol. It can emit versioned JSON fixtures, create
a loopback callback, wait for cancellation, prompt through a PTY, simulate
expiry, and spawn a child to verify process-tree cleanup. It must not reuse
Teleport code or branding in a way that implies a real client.

Use it to test:

- every success and failure state without network access;
- stdout/stderr separation and output bombs;
- browser opener invocation and rejection of injected URLs;
- credential-home isolation and global SSH-agent nonmutation;
- frontend lease loss, app shutdown, cancel races, and repeated wait/close;
- terminal byte fidelity, Unicode, resize, signals, and backpressure; and
- changed executable detection between inspect and execute.

The fake fixture is not evidence of Teleport protocol compatibility. It tests
the app's adapter and lifecycle only.

### 11.3 Real Teleport Integration Tests

Run a separate opt-in suite against an administrator-provided or otherwise
legally approved Teleport environment. Do not redistribute a Community Edition
server image in CI until its terms have been reviewed for that exact use.

The matrix covers:

- each supported client major against same-major and documented older-client
  server combinations;
- root and leaf clusters;
- local auth, SSO, TOTP/WebAuthn, and per-session MFA where automatable;
- short certificate expiry and explicit reauthentication;
- allow, deny, node removal, role change, and access-request transitions;
- session recording enabled and disabled by cluster policy;
- proxy TLS with public and administrator-installed private roots;
- proxy/network interruption during login and active terminal use; and
- logout plus local identity verification.

Human release checks cover identity-provider pages and hardware-key ceremonies
that should not be automated with production credentials.

### 11.4 Platform and Security Tests

On each platform verify:

- external browser launch and focus behavior;
- callback listener bound only to loopback;
- no unexpected listening socket after cancel/logout/exit;
- exact executable selected despite hostile <code>PATH</code> and working
  directory;
- process-tree cleanup after frontend crash, app close, and forced child hang;
- private credential/store permissions and backup-exclusion behavior;
- no token/certificate/browser URL in logs, events, crash data, or diagnostics;
- no new identity in the global SSH agent; and
- layout at compact and large window sizes with long cluster/node/label text.

## 12. Release and Operations

The feature begins behind an explicit experimental flag and is off for existing
users. Enabling it explains that a compatible local <code>tsh</code> is
required and that the Teleport administrator controls authentication, access,
and recording.

Release artifacts are scanned to prove they contain no <code>tsh</code>,
Teleport server, download URL, managed-update bootstrap, private fixture, or
copied identity material. Dependency and notice generation must not imply that
invoking an external client makes it part of the application's dependency
graph, while the user documentation still names the operational prerequisite
clearly.

Administrator documentation covers:

- supported Teleport/client versions;
- installing <code>tsh</code> through an authorized organizational method;
- trusted proxy certificates and private CA deployment;
- optional executable path/digest/signer policy;
- identity directory location and logout behavior;
- SSO callback/firewall expectations for local loopback;
- session-recording and audit responsibility; and
- known unsupported Teleport resource types.

Compatibility is revalidated before adding a new Teleport major. A release may
drop an old major only with a documented support window and clear upgrade path.
The app never solves version drift by bypassing client checks.

## 13. Acceptance Criteria

The initial Teleport terminal release is complete only when all of the
following are true:

- A cluster can be added with a canonical proxy and selected absolute
  <code>tsh</code> path without authenticating.
- The executable is inspected, change-detected, and invoked without a shell.
- No official Teleport binary or server artifact is bundled or downloaded.
- Proxy discovery uses verified TLS and provides no insecure bypass.
- Unsupported client/server versions fail before login or terminal creation.
- Every cluster uses a separate private app-owned
  <code>TELEPORT_HOME</code>.
- The user's normal <code>~/.tsh</code> profiles and global SSH agent are
  unchanged.
- Managed client download/re-execution is disabled by default.
- Browser authentication opens in the external system browser from a validated
  active-login URL.
- Callback listeners are loopback-only and close on success, failure, cancel,
  lease expiry, and app exit.
- Passwords, MFA values, assertions, tokens, certificates, and browser URLs are
  absent from persistent state, logs, diagnostics, and frontend events.
- Login success is verified through fresh structured status, not output text.
- Certificate expiry, roles, allowed logins, and active cluster are visible.
- Root and leaf cluster nodes are discovered with bounded strict parsing.
- A saved target re-resolves and re-authorizes before every connection.
- <code>tsh ssh</code> runs under a real PTY with resize, signals, Unicode,
  mouse-capable terminal programs, backpressure, exit status, and deterministic
  cleanup.
- Per-session MFA and server denial remain controlled by Teleport.
- The terminal visibly identifies Teleport policy and possible recording.
- Local session logging remains separately opt-in.
- Restore creates disconnected tabs and performs no background login/network
  action.
- Logout verifies local identity removal and never overstates success.
- Forget/delete cannot remove data outside the app-owned Teleport directory.
- Malicious arguments, output, URLs, labels, executable replacement, and stale
  generations are covered by automated tests.
- No login, terminal, listener, or descendant process survives app shutdown.
- The advertised platform and real-cluster compatibility matrix passes.
- Legal, security, privacy, and trademark release gates are recorded.

## 14. Explicit Non-Goals

The initial implementation does not:

- install or administer Teleport Proxy, Auth Service, agents, or node services;
- install <code>tsh</code> automatically on the local computer or target nodes;
- embed or directly import Teleport's full client implementation;
- use the Apache-licensed API module as a claim that all client code is Apache
  licensed;
- bypass TLS, version checks, MFA, RBAC, access requests, or session recording;
- convert Teleport nodes into direct ordinary SSH profiles;
- use a node's address to route around the Teleport Proxy;
- expose arbitrary CLI flags, config files, identity files, or environment
  variables;
- provide background auto-login or certificate renewal at app startup;
- support application, database, Kubernetes, desktop, or Machine Identity
  resources in the first slice;
- claim SFTP, forwarding, or Remote Projects support before those milestones;
- automate approval of elevated access; or
- replace the organization's Teleport audit, support, or security policy.

## 15. Required ADRs Before Implementation

TP0 must produce at least these decisions:

1. **External Teleport client boundary**: accept the optional local
   <code>tsh</code> prerequisite and document why a native or bundled client
   is not selected.
2. **License and distribution mode**: record the exact reviewed versions,
   terms, artifact source, trademark use, and release audit.
3. **Supported-version policy**: define Teleport majors, compatibility window,
   update suppression, and deprecation process.
4. **Identity isolation**: define <code>TELEPORT_HOME</code>, permissions,
   backup behavior, agent policy, logout, and guarded deletion.
5. **External-browser authentication**: define accepted URL forms, callback
   binding, timeout, cancellation, and token non-observability.
6. **Teleport terminal transport**: define exact PTY execution, process
   ownership, MFA, recording disclosure, and workspace restore behavior.
7. **Future capabilities**: separately decide access requests, transfer,
   forwarding, and Teleport-backed Remote Projects after their spikes.

The official references used by TP0 include the
[tsh client guide](https://goteleport.com/docs/connect-your-client/teleport-clients/tsh/),
[tsh command reference](https://goteleport.com/docs/reference/cli/tsh/),
[TLS routing architecture](https://goteleport.com/docs/reference/architecture/tls-routing/),
and
[OpenSSH integration documentation](https://goteleport.com/docs/enroll-resources/server-access/openssh/openssh-manual-install/).
Each ADR records the exact document version/date because CLI flags, behavior,
compatibility, and licensing can change.
