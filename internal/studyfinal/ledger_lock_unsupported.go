//go:build !darwin && !linux

package studyfinal

import (
	"fmt"
	"os"
)

func lockLedgerFile(*os.File) error {
	return fmt.Errorf("execution ledger locking is supported only on macOS and Linux")
}

func unlockLedgerFile(*os.File) error { return nil }
