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

## Reviewed Module-Only Advisory

The verbose scan on 2026-07-17 reported `GO-2026-5932` against
`golang.org/x/crypto@v0.54.0`. The advisory concerns the unmaintained
`golang.org/x/crypto/openpgp` package. This application imports the maintained
SSH packages from that module and does not import or call `openpgp`; the same
scan reported zero package-level and zero symbol-level vulnerabilities.

This note is not a permanent waiver. Re-run the scan after dependency changes
and before a release, and remove or revise the decision if reachability or the
upstream advisory changes.
