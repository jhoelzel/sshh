# Terminal Lifecycle Stress Verification

This document records the automated terminal lifecycle evidence separately
from the native WebView performance benchmark. The lifecycle suite is intended
to fail on leaked shells, process descendants, terminal runtimes, goroutines,
file descriptors, controllers, or Wails event subscriptions.

## Automated Gate

### Frontend replacement

`Desktop.AttachFrontend` serializes attachment attempts. Reattaching the same
frontend nonce renews its existing lease. Attaching a different nonce first
invalidates the previous lease, then waits for all terminal, SFTP, and tunnel
resources owned by that lease to close before returning the replacement lease.
The independent resource managers close concurrently so one slow resource does
not add its full shutdown budget to every other manager.

`TestAttachFrontendReplacesPreviousInstanceAndReapsItsRuntime` opens and
activates a terminal under the first lease, attaches a different frontend, and
proves that the old transport is closed and the terminal runtime count is zero
before the replacement command returns. The old lease is stale immediately;
the new lease remains renewable.

### PTY flood and close

`TestManagerClosesRealPTYFloodWithoutResourceLeaks` runs on Darwin and Linux
against the real Unix PTY adapter. It performs one warm-up cycle and then two
measured cycles for each production close path:

- tab close through `Manager.Close`;
- application close through `Manager.Shutdown`;
- 2 MiB of output before every measured close;
- a shell descendant whose PID is recorded and which ignores hangup, so cleanup
  cannot rely on a cooperative shell;
- cumulative output acknowledgements without storing terminal content; and
- a three-second per-close test budget, below the five-second application
  shutdown budget.

After every measured cycle, the descendant PID must no longer exist, the
manager runtime count must be zero, and process-wide goroutine and descriptor
counts must be no higher than the post-warm-up baseline. Descriptor counts come
from name-only reads of `/dev/fd` on macOS and `/proc/self/fd` on Linux.

### Short-lived PTY churn

`TestManagerReaps100ShortLivedRealPTYsWithoutResourceLeaks` runs on Darwin and
Linux against the production manager and real Unix PTY adapter. After one
warm-up, a single manager opens and activates 100 short-lived `/bin/sh`
sessions in sequence. Every session must deliver acknowledged output, report a
zero exit status, transition through explicit tab closure, and leave no runtime
or output dispatcher registered before the next session opens.

After all 100 measured cycles, process-wide goroutine and descriptor counts
must be no higher than their post-warm-up baselines. The same workload runs
under the Go race detector, covering concurrent process wait, output delivery,
acknowledgement, state publication, and cleanup.

### React and bridge listeners

`App.strictmode.test.tsx` renders the complete application in React StrictMode,
opens one terminal, and delivers 512 terminal output events. Closing the tab
disposes its controller and later events for that session are ignored while the
application's six shared subscriptions remain active. Unmounting the root
returns all six subscriptions to zero. A genuine second mount repeats one
bootstrap, one terminal open, one controller, the output burst, and complete
disposal without retaining callbacks from the first mount.

## Current Mac Record

The focused gates passed on 2026-07-18 with Go 1.26.5 on an arm64 Mac running
macOS 26.5.2:

```sh
go test ./internal/usecase/session \
  -run TestManagerClosesRealPTYFloodWithoutResourceLeaks -count=1
go test ./internal/usecase/session \
  -run TestManagerReaps100ShortLivedRealPTYsWithoutResourceLeaks -count=1 -v
go test -race ./internal/usecase/session \
  -run TestManagerReaps100ShortLivedRealPTYsWithoutResourceLeaks -count=1 -v
go test ./internal/bridge -run TestAttachFrontend -count=1
cd frontend && npm test -- --run src/app/App.strictmode.test.tsx
```

The PTY flood test completed its warm-up and four measured cycles in 5.07
seconds. The short-lived PTY loop completed 100 measured sessions in 478 ms
normally and 574 ms under the race detector. Both runs delivered 4,000 measured
output bytes and returned to 2/2 goroutines and 6/6 descriptors relative to
their warmed baselines. These timings are test-run observations, not rendering
throughput budgets.

## Native Performance Gate

The separate packaged WKWebView harness now exercises Wails serialization,
xterm parsing, a capped scrollback buffer, input, resize, close under output,
queue high-water marks, and app/fixture/WebKit RSS with a 10 MiB PTY fixture. The
recorded run passes the M1 provisional budgets. Its implementation, reproduction
command, measurements, and remaining scope are documented in
[`TERMINAL_BENCHMARK.md`](TERMINAL_BENCHMARK.md).

The packaged 15-minute, eight-session follow-up also proves bounded
steady-state RSS growth, sustained input responsiveness, exact queue drain, and
native cleanup after 439.5 MiB of aggregate output. Its workload, budgets, and
machine record are documented in [`TERMINAL_SOAK.md`](TERMINAL_SOAK.md).

Pixel-paint timing, IME behavior, sleep/wake timer suspension, multi-hour
stability, and larger concurrent-session counts remain outside these gates.
