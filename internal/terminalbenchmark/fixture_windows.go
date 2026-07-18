//go:build windows

package terminalbenchmark

import (
	"errors"
	"os"
)

func runFixture(*os.File, *os.File) error {
	return errors.New("terminal benchmark fixture requires a Unix pseudoterminal")
}
