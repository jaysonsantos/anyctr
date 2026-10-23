//go:build windows

package procexec

import "os"

// forwardSignals is a no-op on Windows: console control events are delivered to
// the whole process group, so the child already receives Ctrl-C directly, and
// Windows has no signal to relay for the rest.
func forwardSignals(proc *os.Process) func() {
	return func() {}
}
