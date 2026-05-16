package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// sweepOrphanPIDPrefixedDirs removes stale test fixture directories whose
// suffix encodes a PID that is no longer alive.
func sweepOrphanPIDPrefixedDirs(root, prefix string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	self := os.Getpid()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
		if err != nil || pid <= 0 || pid == self {
			continue
		}
		if pidAlive(pid) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, name))
	}
}
