// Package procexec runs a translated invocation as a child process.
//
// anyctr is a gateway, not a supervisor: the child inherits the terminal
// directly, so TTY allocation, colour, progress bars and interactive shells all
// behave exactly as if the engineer had typed the runtime's command themselves.
// Signals are forwarded, and the child's exit status becomes anyctr's.
package procexec

import (
	"errors"
	"os"
	"os/exec"

	"github.com/pedrospdc/anyctr/internal/runtime"
)

// ExitCodeError carries a child process exit status.
type ExitCodeError struct {
	Code int
}

func (e *ExitCodeError) Error() string { return "child exited with status " + itoa(e.Code) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// Run executes the invocation, wiring the child straight to this process's
// stdio. It returns the child's exit code.
func Run(inv runtime.Invocation) (int, error) {
	bin := inv.Bin
	if !isPath(bin) {
		resolved, err := exec.LookPath(bin)
		if err != nil {
			return 125, err
		}
		bin = resolved
	}

	cmd := exec.Command(bin, inv.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 125, err
	}

	stop := forwardSignals(cmd.Process)
	defer stop()

	err := cmd.Wait()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 125, err
}

func isPath(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' || s[i] == '\\' {
			return true
		}
	}
	return false
}
