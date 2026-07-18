# ADR 0003: Native Terminal Soak Budgets

- Status: Accepted
- Date: 2026-07-18

## Context

The short M1 benchmark proves burst throughput and responsiveness but cannot
show whether saturated xterm scrollbacks, Wails event delivery, PTY fixtures,
or WebKit retain memory over time. A useful native soak must exercise multiple
persistent terminal controllers, including inactive tabs, while continuing to
measure input and deterministic cleanup.

Whole-application memory includes the packaged app, fixture descendants, and
baseline-differenced WebKit XPC helpers. A 30-second diagnostic observed an
805,240,832-byte startup peak for eight terminals, so the earlier single-session
512 MiB budget and an initial 768 MiB proposal did not provide honest allocator
headroom for this different workload.

## Decision

The packaged macOS soak uses these fixed requirements:

- eight concurrent terminals for at least 15 minutes;
- a 16,000-byte output chunk per terminal every 250 ms;
- at least 1,200 measured echoes and 120 active-terminal rotations;
- 150 ms input echo p95;
- 1 second maximum close time for every session;
- 1 MiB backend and frontend flow-control windows per session;
- at least 75 percent of the scheduled payload per session;
- 1 GiB peak resident memory across the app, fixtures, and new WebKit helpers;
- no more than 96 MiB positive median RSS growth between minutes one through
  two and the final minute; and
- at least 600 one-second RSS samples, with at least 30 samples in each median
  window.

The peak ceiling is a regression envelope for unavoidable process and WebKit
startup cost. The smaller steady-state growth ceiling is the primary retention
guard. Both must pass. Queue drain, exact sequence and byte equality, parser
health, fixture presence, WebKit presence, and zero live runtimes are also hard
requirements.

## Consequences

The accepted run has 858,210,304 bytes peak RSS but only 19,087,360 bytes of
warmed-to-final growth after moving 460,844,584 bytes through eight terminals.
This supports bounded retention without pretending that eight native WebView
terminal instances are inexpensive at startup.

The result applies to one Apple Silicon Mac and a 15-minute, eight-session
workload. Multi-hour runs, larger session counts, alternate renderers, other
hardware, Linux WebKitGTK, and Windows WebView2 require separate evidence. Any
change to an accepted budget requires a new measured report and an ADR update
or successor.
