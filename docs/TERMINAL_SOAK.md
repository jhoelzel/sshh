# Native Terminal Soak Verification

This document records the reproducible long-duration terminal gate across the
complete packaged macOS path:

```text
8 PTYs -> Go session manager -> Wails event bridge -> WKWebView -> 8 xterm controllers
       <- ordered input, tab visibility changes, and close commands <-
```

The benchmark uses the production Wails package and system WKWebView. Seven
terminal hosts are hidden with the same persistent-host behavior as inactive
product tabs, while one host rotates into view every five seconds.

## Reproduce

Run from the repository root on macOS:

```sh
./scripts/run-terminal-soak-macos.sh
```

The command builds an explicitly flagged benchmark bundle, runs it with an
isolated temporary home, writes
`docs/benchmarks/m1-macos-arm64-soak.json`, and restores the ordinary product
bundle even when a budget fails. The accepted duration and workload are fixed
in the backend; there is no environment override that can shorten a passing
run.

The benchmark bridge remains inert in ordinary launches. Its same-binary PTY
fixture is enabled only by guarded launch variables and a validated report path
inside the system temporary directory. Reports and phase checkpoints contain
only timing, sequence, byte, queue, process, and hardware counters. Terminal
payloads are never recorded.

## Workload

The accepted run was recorded on 2026-07-18 with:

- Mac model `Mac16,7`, Apple M4 Pro, 48 GiB memory;
- macOS 26.5.2 build 25F84, arm64;
- Go 1.26.5 and Wails 2.13.0;
- xterm.js 6.0.0 using its standard renderer; and
- eight 100 by 30 terminals, each capped at 10,000 scrollback lines.

Each PTY emits a 16,000-byte line-oriented chunk every 250 ms for 15 minutes.
Every five seconds the WebView rotates the active terminal and sends a unique
echo request through all eight ordered input queues. After 180 cycles it stops
all fixtures, waits for every frontend and backend queue to drain, captures
per-session diagnostics, closes all eight sessions, and completes the report
only after the backend runtime count reaches zero.

The host samples resident memory once per second. It combines the app and its
eight fixture descendants with WebKit helpers that appeared after launch. The
median from minutes one through two is the warmed baseline; the median from the
final minute is the ending value. Positive growth between those medians catches
sustained retention without treating startup allocation as a leak.

## Accepted Result

| Measurement | Provisional budget | Recorded result |
| --- | ---: | ---: |
| Duration | at least 900,000 ms | 900,001 ms |
| Concurrent sessions | exactly 8 | 8 |
| Visibility rotations | at least 120 | 180 |
| Input echo samples | at least 1,200 | 1,440 |
| Input echo p95 | 150 ms | 11 ms |
| Close p95 and per-session maximum | 1,000 ms | 2 ms |
| Payload per session | at least 43,200,000 bytes | 57,605,573 bytes |
| Aggregate terminal bytes | at least 345,600,000 bytes | 460,844,584 bytes |
| Backend unacknowledged bytes per session | 1 MiB | 29,312 bytes peak |
| Frontend pending writes per session | 1 MiB | 17,664 bytes peak |
| App, fixtures, and WebKit peak RSS | 1 GiB | 858,210,304 bytes |
| Warmed-to-final median RSS growth | 96 MiB | 19,087,360 bytes |
| Whole-application RSS samples | at least 600 | 901 |

The sampler observed twelve processes at peak: the app, eight fixture
processes, and three WebKit helpers. Every controller and backend finished with
identical sequence and byte counters, every queue drained to zero, and no xterm
parser failed. All eight sessions closed in one or two milliseconds.

The machine-readable evidence is
[`benchmarks/m1-macos-arm64-soak.json`](benchmarks/m1-macos-arm64-soak.json).

## Scope

This closes the planned 15-minute, eight-session packaged-macOS terminal soak
and native multi-session stress gate on the recorded hardware. It does not
claim multi-hour stability, larger concurrent-session counts, TUI or IME
correctness, sleep/wake behavior, another Mac model, or native Linux and Windows
performance. Those remain separate plan gates.
