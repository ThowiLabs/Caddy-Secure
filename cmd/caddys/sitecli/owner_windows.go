//go:build windows

package sitecli

import "os"

func ownerIDs(info os.FileInfo) (int, int) { return -1, -1 }
