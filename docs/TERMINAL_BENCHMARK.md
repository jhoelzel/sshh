# Native Terminal Performance Benchmark

This document records the reproducible M1 terminal performance gate across the
complete packaged path:

```text
macOS PTY -> Go session manager -> Wails event bridge -> WKWebView -> xterm.js
          <- ordered input and debounced resize commands <-
```

The benchmark uses a production-mode Wails application package and the system
WKWebView. It is not a jsdom or browser-only throughput test.

## Reproduce

Run from the repository root on macOS:

```sh
./scripts/run-terminal-benchmark-macos.sh
```

The script builds an explicitly flagged benchmark bundle, launches the packaged
application with an isolated temporary home directory, records
`docs/benchmarks/m1-macos-arm64.json`, and rebuilds the ordinary product bundle
before returning. A failed budget produces a nonzero exit status while still
restoring the ordinary bundle.

The benchmark bridge is inert unless both launch-only environment variables are
present and the report path resolves inside the system temporary directory. The
PTY fixture is a fixed mode of the same `shhh` executable, not a downloaded or
shipped sidecar. The normal frontend build excludes the benchmark component.
Diagnostics contain only timing, sequence, byte, queue, process, and hardware
counters. Terminal payloads are never included in the report.

## Recorded Run

The accepted run was recorded on 2026-07-18 with:

- Mac model `Mac16,7`, Apple M4 Pro, 48 GiB memory;
- macOS 26.5.2 build 25F84, arm64;
- Go 1.26.5 and Wails 2.13.0;
- xterm.js 6.0.0 using its standard renderer; and
- a 100 by 30 terminal with a 10,000-line scrollback cap.

The fixture writes exactly 10 MiB as 80-byte CRLF-terminated lines. Twenty
round trips are measured for idle input, input during the flood, and resize.
Resize timing includes the production controller's 80 ms debounce. After the
main stream drains, a continuing output stream verifies close responsiveness.
Before launch, the host runner records every existing WebKit XPC PID. Every
50 ms it then samples the packaged process and descendants plus newly observed
WebKit GPU, networking, and content helpers. The accepted peak contains five
processes: the app, its PTY fixture, and three WebKit helpers. Baseline exclusion
avoids charging unrelated, already-running WebViews to this application. A
different application starting a new WebKit helper during the short run would
be conservatively included rather than hidden from the total.

| Measurement | Provisional budget | Recorded result |
| --- | ---: | ---: |
| 10 MiB completion | 10,000 ms | 421 ms |
| Idle input echo p95 | 50 ms | 1 ms |
| Flood input echo p95 | 150 ms | 26 ms |
| Resize p95 | 150 ms | 83 ms |
| Close during output | 1,000 ms | 11 ms |
| Backend unacknowledged bytes | 1 MiB | 1,048,220 bytes peak |
| Frontend pending writes | 1 MiB | 748,288 bytes peak |
| App, fixture, and WebKit RSS | 512 MiB | 434,814,976 bytes peak |

The final backend and controller counters both reached sequence 10,546 and byte
offset 10,487,510. The additional 1,750 bytes are fixed title/control markers.
Both queue counters drained to zero, xterm reported no parser failure, and every
accepted byte was consumed and cumulatively acknowledged. The result therefore
proves ordering and loss detection as well as timing.

The machine-readable evidence is
[`benchmarks/m1-macos-arm64.json`](benchmarks/m1-macos-arm64.json).

## Scope

This closes the M1 packaged-macOS throughput, queue, scrollback-memory, input,
resize, completion, and close-response gate on the recorded hardware. It does
not claim pixel-paint timing, IME correctness, sleep/wake lease behavior,
another Mac model, or native Linux and Windows performance. The separate
15-minute, eight-session native gate is recorded in
[`TERMINAL_SOAK.md`](TERMINAL_SOAK.md); multi-hour stability remains open.
