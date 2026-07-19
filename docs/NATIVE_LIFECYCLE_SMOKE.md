# Native Wails Lifecycle Smoke

## Purpose

The lifecycle smoke verifies the packaged desktop path that unit and browser
tests cannot prove. It exercises Wails' native close callback, the frontend
decision event, coordinated backend shutdown, the second permitted close, and
the final `OnShutdown` hook while a real PTY is active.

This is a content-free test. Its JSON report contains hook booleans, bounded
terminal counts, timestamps, process counts, runtime identity, and failure
messages. It does not contain terminal output, input, environment values,
profile data, paths outside the isolated test directory, or credentials.

## Sequence

1. The external host creates an isolated home and private temporary report path.
2. The benchmark-only frontend attaches and opens the same executable with its
   guarded PTY-fixture argument.
3. After xterm observes the fixture-ready marker, the frontend calls Wails
   `Quit` while exactly one terminal is live.
4. The first `OnBeforeClose` must return `true`, retain the terminal, and emit
   `shhh:close-requested`.
5. The frontend remains alive for at least 200 ms, records the decision, and
   calls the ordinary `ConfirmApplicationClose` bridge command.
6. Coordinated shutdown closes the PTY before the second native close. That
   `OnBeforeClose` must return `false` with zero live terminals.
7. Wails invokes `OnShutdown`; the application closes its remaining services and
   atomically finalizes the private report.
8. The external host requires process samples containing the application, PTY
   fixture child, and at least one WKWebView or WebKitGTK helper.

Any missing hook, unexpected extra close attempt, early frontend destruction,
failed cleanup, absent process evidence, or report timeout fails the gate.

## macOS Reproduction

Run from the repository root on macOS arm64:

```sh
./scripts/run-lifecycle-smoke-macos.sh
```

The script builds the guarded packaged frontend, runs the smoke, writes
`m3-macos-arm64-lifecycle.json` under the system temporary directory, and then
restores the ordinary product bundle even when the smoke fails. An optional
absolute report path may be supplied as the first argument.

The accepted 2026-07-19 Apple Silicon run observed two native close attempts, a
262 ms decision interval, one live terminal retained by the first close, zero
terminals before the second close, completed startup/DOM-ready/shutdown hooks,
30 process samples, and three WKWebView helpers.

## Linux CI

Ubuntu 24.04 CI builds the existing guarded WebKitGTK host once and launches its
terminal interaction and lifecycle modes separately under Xvfb. The lifecycle
run additionally enforces WebKitGTK 2.41 or newer through the 4.1 ABI. Its report
is printed into the job log whether the gate passes or fails.

## Scope

The smoke proves native Wails lifecycle behavior on the current packaged macOS
development platform and Linux CI baseline. It does not replace Windows
WebView2 keyboard, AltGr, IME, clipboard, or accessibility validation; those
remain explicit release gates. It also does not perform release signing or
notarization.
