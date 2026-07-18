package terminalbenchmark

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MarkerReady        = "SHHH_BENCH_READY"
	MarkerEchoPrefix   = "SHHH_BENCH_ECHO:"
	MarkerRenderProbe  = "SHHH_BENCH_RENDER_PROBE"
	MarkerResize       = "SHHH_BENCH_RESIZE:"
	MarkerDone         = "SHHH_BENCH_DONE:"
	MarkerCloseFlood   = "SHHH_BENCH_CLOSE_FLOOD"
	MarkerSoakStarted  = "SHHH_BENCH_SOAK_STARTED"
	MarkerSoakDone     = "SHHH_BENCH_SOAK_DONE"
	commandFlood       = "FLOOD:"
	commandRenderProbe = "RENDER_PROBE"
)

const renderProbeLine = "render-probe.................................................................\r\n"

func writeRenderProbe(output io.Writer, writes *sync.Mutex) error {
	writes.Lock()
	defer writes.Unlock()
	if err := writeAll(output, []byte(strings.Repeat(renderProbeLine, 1_024))); err != nil {
		return err
	}
	return writeAll(output, []byte("\x1b]0;"+MarkerRenderProbe+"\x07"))
}

func RunFixtureIfRequested(arguments []string) (bool, error) {
	if len(arguments) != 1 || arguments[0] != FixtureArgument {
		return false, nil
	}
	if os.Getenv(EnvironmentFixture) != "1" {
		return true, errors.New("terminal benchmark fixture is not authorized")
	}
	return true, runFixture(os.Stdin, os.Stdout)
}

func writeSoak(
	output io.Writer,
	writes *sync.Mutex,
	stop <-chan struct{},
	title func(string) error,
) {
	if title(MarkerSoakStarted) != nil {
		return
	}
	line := []byte(strings.Repeat("s", 78) + "\r\n")
	chunk := make([]byte, 0, SoakOutputChunkBytes)
	for len(chunk)+len(line) <= cap(chunk) {
		chunk = append(chunk, line...)
	}
	ticker := time.NewTicker(SoakOutputInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			_ = title(MarkerSoakDone)
			return
		case <-ticker.C:
			writes.Lock()
			err := writeAll(output, chunk)
			writes.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func writeFlood(output io.Writer, writes *sync.Mutex, byteCount uint64, complete func()) {
	line := []byte(strings.Repeat("x", 78) + "\r\n")
	chunk := make([]byte, 0, 16_000)
	for len(chunk)+len(line) <= cap(chunk) {
		chunk = append(chunk, line...)
	}
	remaining := byteCount
	for remaining > 0 {
		data := chunk
		if uint64(len(data)) > remaining {
			data = data[:remaining]
		}
		writes.Lock()
		err := writeAll(output, data)
		writes.Unlock()
		if err != nil {
			return
		}
		remaining -= uint64(len(data))
		time.Sleep(250 * time.Microsecond)
	}
	complete()
}

func parseFloodCommand(command string) (uint64, bool) {
	value, found := strings.CutPrefix(command, commandFlood)
	if !found {
		return 0, false
	}
	byteCount, err := strconv.ParseUint(value, 10, 64)
	if err != nil || byteCount == 0 || byteCount > PayloadBytes {
		return 0, false
	}
	return byteCount, true
}

func writeCloseFlood(output io.Writer, writes *sync.Mutex, title func(string) error) {
	if title(MarkerCloseFlood) != nil {
		return
	}
	chunk := []byte(strings.Repeat("close-pressure\n", 1_024))
	for {
		writes.Lock()
		err := writeAll(output, chunk)
		writes.Unlock()
		if err != nil {
			return
		}
	}
}

func writeAll(output io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := output.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
