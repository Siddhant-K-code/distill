//go:build darwin || linux

package studyfinal

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockLedgerFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}

func unlockLedgerFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
