//go:build !windows

package procexec

import (
	"os"
	"os/signal"
	"syscall"
)

// forwardSignals relays terminal signals to the child, so Ctrl-C reaches the
// container runtime rather than killing the gateway out from under it.
func forwardSignals(proc *os.Process) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-ch:
				_ = proc.Signal(sig)
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}
