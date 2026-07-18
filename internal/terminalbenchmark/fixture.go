//go:build darwin || linux

package terminalbenchmark

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/term"
)

func runFixture(input *os.File, output *os.File) error {
	state, err := term.MakeRaw(int(input.Fd()))
	if err != nil {
		return fmt.Errorf("enable raw benchmark terminal: %w", err)
	}
	defer term.Restore(int(input.Fd()), state)

	var writes sync.Mutex
	writeTitle := func(title string) error {
		writes.Lock()
		defer writes.Unlock()
		return writeAll(output, []byte("\x1b]0;"+title+"\x07"))
	}
	if err := writeTitle(MarkerReady); err != nil {
		return err
	}

	resized := make(chan os.Signal, 1)
	signal.Notify(resized, syscall.SIGWINCH)
	defer signal.Stop(resized)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-resized:
				columns, rows, sizeErr := term.GetSize(int(output.Fd()))
				if sizeErr == nil {
					_ = writeTitle(fmt.Sprintf("%s%dx%d", MarkerResize, columns, rows))
				}
			}
		}
	}()

	reader := bufio.NewReader(input)
	var floodOnce sync.Once
	var closeFloodOnce sync.Once
	var soakOnce sync.Once
	var stopSoakOnce sync.Once
	stopSoak := make(chan struct{})
	defer stopSoakOnce.Do(func() { close(stopSoak) })
	for {
		command, readErr := reader.ReadString('\r')
		if readErr != nil {
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, syscall.EIO) {
				return nil
			}
			return fmt.Errorf("read benchmark command: %w", readErr)
		}
		command = strings.TrimSuffix(command, "\r")
		switch {
		case strings.HasPrefix(command, "PING:"):
			if err := writeTitle(MarkerEchoPrefix + strings.TrimPrefix(command, "PING:")); err != nil {
				return err
			}
		case command == "FLOOD":
			floodOnce.Do(func() {
				go writeFlood(output, &writes, PayloadBytes, func() {
					_ = writeTitle(fmt.Sprintf("%s%d", MarkerDone, PayloadBytes))
				})
			})
		case command == "CLOSE_FLOOD":
			closeFloodOnce.Do(func() {
				go writeCloseFlood(output, &writes, writeTitle)
			})
		case command == "SOAK":
			soakOnce.Do(func() {
				go writeSoak(output, &writes, stopSoak, writeTitle)
			})
		case command == "STOP_SOAK":
			stopSoakOnce.Do(func() { close(stopSoak) })
		case command == "EXIT":
			return nil
		}
	}
}
