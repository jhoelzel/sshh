# Linux Desktop Support

shh-h supports native local terminals on Linux through the Unix PTY adapter and
uses Wails' GTK 3/WebKitGTK desktop host. The application frontend, Go backend,
and same-binary PTY fixture are embedded in one application executable. GTK and
WebKitGTK remain operating-system libraries; shh-h does not bundle a private
browser runtime or claim to be a statically linked binary.

## Supported Baseline

The native CI baseline is Ubuntu 24.04 amd64 with:

- GTK 3;
- the `webkit2gtk-4.1` ABI;
- WebKitGTK 2.41.0 or newer; and
- a POSIX shell at `/bin/sh` for the native test fixtures.

Linux builds pass Wails' `webkit2_41` build tag. In Wails 2.13.0 this selects
the WebKitGTK 4.1 pkg-config target and performs a startup check for WebKitGTK
2.41 or newer. CI separately runs `pkg-config --atleast-version=2.41.0
webkit2gtk-4.1`, so an image below the documented floor fails before the app is
built or launched.

End-user packages must declare GTK 3 and a compatible WebKitGTK 4.1 runtime as
system dependencies. AppImage, deb, rpm, distribution coverage, and desktop
integration remain M11 packaging work; the current gate proves the raw native
executable on the baseline above.

## Native Gate

The `native-linux` CI job performs three independent checks:

1. The real Linux PTY tests verify initial dimensions, exact environment
   delivery, binary input, live resize, exit status, close during a 2 MiB output
   flood, descendant process-group termination, descriptor/goroutine cleanup,
   and 100 short-lived terminal cycles.
2. The ordinary production Wails host compiles against the WebKitGTK 4.1 ABI.
3. A benchmark-flagged production host launches under a private D-Bus session
   and Xvfb. It restores focus from an external input to the real xterm control,
   performs and restores a native clipboard round trip, and hides the real
   terminal while a line-oriented render probe crosses the PTY and Wails bridge.
   The xterm render count must remain unchanged while hidden and advance after
   the host is restored. The smoke then streams 128 KiB through the same-executable
   PTY child, performs five live resizes, closes under output, and records the
   app, PTY child, and WebKitGTK helper process tree. The smaller stream keeps
   this a functional gate under Xvfb; the PTY suite separately closes during a
   2 MiB flood and macOS retains the 10 MiB performance gate. Smoke and
   performance modes send an explicit bounded byte count to the same fixture
   protocol, which rejects zero, malformed, and greater-than-10-MiB requests.

The smoke report contains only booleans, timing, byte/sequence counters, queue
high-water marks, process counts, host facts, and the WebKitGTK version. It does
not contain terminal output or clipboard contents. Clipboard state is restored
before the check completes.

The smoke mode has functional pass criteria and deliberately does not reuse the
macOS performance budgets. Shared CI machines are suitable for proving native
integration, ordering, drain, and cleanup, but not for establishing portable
latency or memory thresholds.

## Local Reproduction

Install the development packages that provide GTK 3, `webkit2gtk-4.1`,
pkg-config, D-Bus, Xauth, and Xvfb. On Ubuntu 24.04:

```sh
sudo apt-get install libgtk-3-dev libwebkit2gtk-4.1-dev dbus-x11 xauth xvfb
pkg-config --atleast-version=2.41.0 webkit2gtk-4.1
go test -count=1 -timeout=5m ./internal/adapter/localpty ./internal/usecase/session
make bootstrap-wails
cd cmd/shhh
VITE_TERMINAL_BENCHMARK=1 ../../bin/wails build -clean -nopackage \
  -trimpath -tags webkit2_41 -o shhh-linux-smoke
cd ../..
LIBGL_ALWAYS_SOFTWARE=1 WEBKIT_DISABLE_COMPOSITING_MODE=1 \
  dbus-run-session -- xvfb-run -a go run ./cmd/terminalbench \
  -mode smoke -app ./build/bin/shhh-linux-smoke \
  -report "${TMPDIR:-/tmp}/m2-linux-amd64-smoke.json" -timeout 90s
```

The native smoke does not yet prove IME composition, keyboard-layout variants,
forward/reverse tab traversal, desktop portal behavior, Wayland-only behavior,
or a distribution matrix. Those remain explicit interaction and packaging
gates rather than inferred support.
