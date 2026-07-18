# Security Checks

## Automated Gates

The `CI` GitHub Actions workflow runs with read-only repository permissions and
uses actions pinned to immutable full commit SHAs. It performs these checks on
pushes to `main` and on pull requests:

- `go test ./...`, `go test -race ./...`, and `go vet ./...`.
- Frontend lint, unit tests, TypeScript checks, and production compilation after
  a clean `npm ci`.
- The Go team's `govulncheck` action in text mode, which fails when a known
  vulnerability is reachable from the application's call graph.
- `npm audit --audit-level=high`, which fails for high or critical advisories.
- A macOS production-mode Wails compile followed by deterministic generated
  binding comparison.

Dependabot checks Go modules, npm packages, and GitHub Actions weekly. These
checks supplement rather than replace release-time dependency review, native
platform testing, and threat-model review.

## Local Commands

```sh
make test
make lint
make check-bindings
npm --prefix frontend audit --audit-level=high
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

Use `-show verbose` with `govulncheck` during release review to include
module-only findings that are not imported or reachable.

## Profile Environment Values

Local-shell environment overrides are ordinary profile configuration. They use
the same private atomic storage as the rest of a profile and are included in
portable profile exports. They must not contain passwords, tokens, private
keys, or other credentials. shh-h does not include valid override names or
values in its session lifecycle events or diagnostics. Validation errors may
identify an invalid variable name. A launched child process receives valid
overrides and may expose them through its own tools or diagnostics.

Both the frontend and Go domain reject malformed names, case-insensitive name
collisions, null bytes, more than 128 entries, and variables owned by the
terminal runtime. Backend validation remains authoritative for bridge, import,
and manually edited configuration paths.

## Terminal Links

Terminal output is untrusted, including OSC 8 targets and text that resembles a
web address. Both forms pass through the same frontend policy before they can
leave the terminal:

- only absolute `http://` and `https://` URLs are accepted;
- URLs containing credentials, control characters, spaces, backslashes,
  malformed hosts, or more than 2,048 characters are rejected;
- accepted URLs are parsed and canonicalized before display; and
- the exact canonical address requires explicit confirmation and is validated
  again before Wails opens it in the operating system browser.

The application WebView does not navigate to terminal-provided URLs. `file`,
`data`, `javascript`, `ssh`, and other schemes remain inactive. Unit tests cover
both xterm link sources, ambiguous and malicious inputs, canonicalization, and
the confirmation boundary.

## Reviewed Module-Only Advisory

The verbose scan on 2026-07-17 reported `GO-2026-5932` against
`golang.org/x/crypto@v0.54.0`. The advisory concerns the unmaintained
`golang.org/x/crypto/openpgp` package. This application imports the maintained
SSH packages from that module and does not import or call `openpgp`; the same
scan reported zero package-level and zero symbol-level vulnerabilities.

This note is not a permanent waiver. Re-run the scan after dependency changes
and before a release, and remove or revise the decision if reachability or the
upstream advisory changes.
