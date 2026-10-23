package procexec

import (
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/pedrospdc/anyctr/internal/runtime"
)

// The test binary doubles as a fake container runtime: re-executing ourselves is
// the only way to get a predictable child process on every OS without shipping a
// shell script that Windows cannot run.
const (
	fakeRuntimeEnv = "ANYCTR_TEST_FAKE_RUNTIME"
	fakeExitEnv    = "ANYCTR_TEST_FAKE_EXIT"
	argSeparator   = "\x1f"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeRuntimeEnv) != "" {
		fakeRuntime()
		return
	}
	os.Exit(m.Run())
}

// fakeRuntime echoes the argv it was handed and exits with a requested status.
func fakeRuntime() {
	os.Stdout.WriteString(strings.Join(os.Args[1:], argSeparator))
	code, _ := strconv.Atoi(os.Getenv(fakeExitEnv))
	os.Exit(code)
}

// runFake executes the invocation against the fake runtime, returning the argv
// the child actually received and the exit code anyctr reported.
func runFake(t *testing.T, exitCode int, args ...string) ([]string, int, error) {
	t.Helper()
	t.Setenv(fakeRuntimeEnv, "1")
	t.Setenv(fakeExitEnv, strconv.Itoa(exitCode))

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	// procexec wires the child straight to this process's stdio, so capturing the
	// child's output means swapping os.Stdout for a pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w

	code, runErr := Run(runtime.Invocation{Bin: self, Args: args})

	os.Stdout = stdout
	w.Close()
	out, _ := io.ReadAll(r)
	r.Close()

	if len(out) == 0 {
		return nil, code, runErr
	}
	return strings.Split(string(out), argSeparator), code, runErr
}

func TestForwardsArgvVerbatim(t *testing.T) {
	// Quoting and spacing must survive the hop: a volume spec or a shell command
	// mangled here would be a silent corruption of what the engineer typed.
	want := []string{"run", "--rm", "--volume=/a b:/c", "alpine", "sh", "-c", "echo hi"}
	got, code, err := runFake(t, 0, want...)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if len(got) != len(want) {
		t.Fatalf("child received %d args, want %d:\n  got:  %q\n  want: %q", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("arg %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPropagatesExitCode(t *testing.T) {
	// A container that exits 7 must make anyctr exit 7, or every CI script that
	// checks $? downstream of anyctr silently stops working.
	for _, want := range []int{0, 1, 7, 125, 137} {
		got, code, err := runFake(t, want, "run", "alpine")
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if code != want {
			t.Errorf("exit code = %d, want %d (argv %q)", code, want, got)
		}
	}
}

func TestMissingBinaryIsACliError(t *testing.T) {
	code, err := Run(runtime.Invocation{Bin: "anyctr-no-such-runtime-binary"})
	if err == nil {
		t.Fatal("expected an error for a missing binary")
	}
	if code != 125 {
		t.Errorf("exit code = %d, want 125 (anyctr's own failures use Docker's CLI-error status)", code)
	}
}

func TestExitCodeErrorMessage(t *testing.T) {
	for code, want := range map[int]string{0: "0", 7: "7", 137: "137", -1: "-1"} {
		if got := (&ExitCodeError{Code: code}).Error(); !strings.HasSuffix(got, want) {
			t.Errorf("ExitCodeError{%d} = %q, want it to end in %q", code, got, want)
		}
	}
}
