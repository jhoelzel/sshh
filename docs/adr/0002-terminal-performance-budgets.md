# ADR 0002: Native Terminal Performance Budgets

- Status: Accepted
- Date: 2026-07-18

## Context

The M1 plan already fixes queue, completion, input, and resize limits, but it
requires the first packaged-macOS run to record hardware and establish any
additional memory and close-response guardrails before later milestones treat
them as regression budgets. Browser-only tests cannot account for Wails event
serialization, WKWebView, xterm parsing, or operating-system process memory.

## Decision

The packaged benchmark uses these provisional hard limits:

- 1 MiB maximum unacknowledged backend output per terminal;
- 1 MiB maximum pending frontend xterm writes per terminal;
- 10,000 xterm scrollback lines for the benchmark terminal;
- 512 MiB peak resident memory for the packaged app, its fixed PTY fixture, and
  baseline-differenced WebKit XPC helper processes;
- 10 seconds for the exact 10 MiB stream;
- 50 ms idle input echo p95 and 150 ms flood input echo p95;
- 150 ms resize p95, including the 80 ms production debounce; and
- 1 second for tab close while a continuing output stream is active.

The existing five-second coordinated application shutdown limit remains the
absolute lifecycle budget. The one-second benchmark limit is a responsiveness
regression threshold for a healthy local PTY, not a replacement for escalation
windows needed by an uncooperative process.

Resident memory is sampled every 50 ms. The runner combines the root packaged
process and its descendants with WebKit GPU, networking, and content helpers
whose PIDs appear after launch. Existing WebKit helpers are captured as a
baseline and excluded. Whole-application resident memory is intentionally used
instead of Go heap statistics because terminal memory also belongs to WKWebView
and xterm. The 512 MiB ceiling is a conservative regression guard across WebKit
and allocator variation, not a target allocation.

## Consequences

The benchmark report fails if byte or sequence counters differ, either queue
does not drain, a queue cap is exceeded, the xterm parser fails, the fixture
child is not observed, or a timing or memory limit is missed. Reports contain no
terminal payload.

This establishes evidence for one Apple Silicon machine only. Linux WebKitGTK,
Windows WebView2, IME, pixel-paint timing, sleep/wake, multi-hour soak, and
additional hardware still need their own gates. Any change to an accepted
budget requires a new measured report and an amendment or successor ADR.
