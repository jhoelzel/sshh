package terminalbenchmark

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	MarkerReady      = "SHHH_BENCH_READY"
	MarkerEchoPrefix = "SHHH_BENCH_ECHO:"
	MarkerResize     = "SHHH_BENCH_RESIZE:"
	MarkerDone       = "SHHH_BENCH_DONE:"
	MarkerCloseFlood = "SHHH_BENCH_CLOSE_FLOOD"
)

func RunFixtureIfRequested(arguments []string) (bool, error) {
	if len(arguments) != 1 || arguments[0] != FixtureArgument {
		return false, nil
	}
	if os.Getenv(EnvironmentFixture) != "1" {
		return true, errors.New("terminal benchmark fixture is not authorized")
	}
	return true, runFixture(os.Stdin, os.Stdout)
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
