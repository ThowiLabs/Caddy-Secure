//go:build !windows

package sitecli

import (
	"os"
	"syscall"
)

func copyOwnership(existing string, replacement string) error {
	st, err := os.Stat(existing)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Chown(replacement, int(sys.Uid), int(sys.Gid))
}
