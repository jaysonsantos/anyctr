//go:build integration

// Integration tests drive the built anyctr binary against a real container
// runtime, end to end:
//
//	go test -tags integration ./internal/cli/              # the detected runtime
//	ANYCTR_RUNTIME=podman go test -tags integration ./...  # a specific one
//
// Unlike the --explain tests, these prove the runtime *accepts and executes*
// what anyctr produces — and because they run the compiled binary, they also
// cover process execution, exit-code propagation, and runtime detection.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var binary string

// testImage is fully qualified on purpose: podman enforces short-name resolution
// and prompts for a registry when it cannot resolve one, which hangs in CI.
const testImage = "docker.io/library/alpine:latest"

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "anyctr-integration-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	binary = filepath.Join(dir, "anyctr")
	if _, err := os.Stat("../../go.mod"); err != nil {
		fmt.Fprintln(os.Stderr, "integration tests must run from the module:", err)
		os.Exit(1)
	}
	build := exec.Command("go", "build", "-o", binary, "../../cmd/anyctr")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building anyctr:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// anyctr runs the real binary and fails the test if it does not succeed.
func anyctr(t *testing.T, args ...string) string {
	t.Helper()
	out, code := tryAnyctr(t, args...)
	if code != 0 {
		t.Fatalf("anyctr %s exited %d:\n%s", strings.Join(args, " "), code, out)
	}
	return out
}

// tryAnyctr runs the binary and returns its combined output and exit code.
func tryAnyctr(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "ANYCTR_CONFIG="+filepath.Join(t.TempDir(), "config.toml"))
	out, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	if err != nil {
		if ok := asExitError(err, &exitErr); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("running anyctr: %v", err)
		}
	}
	return string(out), code
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("anyctr-it-%s-%d", prefix, time.Now().UnixNano()%1e9)
}

// TestContainerLifecycle is the path an engineer actually walks: pull, run,
// inspect, exec, read logs, stop, remove.
func TestContainerLifecycle(t *testing.T) {
	t.Log(anyctr(t, "version"))
	anyctr(t, "pull", testImage)

	name := uniqueName("life")
	t.Cleanup(func() { tryAnyctr(t, "rm", "-f", name) })

	anyctr(t, "run", "-d", "--name", name, testImage, "sleep", "300")

	if out := anyctr(t, "ps"); !strings.Contains(out, name) {
		t.Errorf("a running container is missing from `anyctr ps`:\n%s", out)
	}

	if out := anyctr(t, "exec", name, "echo", "hello-from-anyctr"); !strings.Contains(out, "hello-from-anyctr") {
		t.Errorf("exec output missing:\n%s", out)
	}

	// Env, workdir and labels have to survive translation to be worth anything.
	envName := uniqueName("env")
	t.Cleanup(func() { tryAnyctr(t, "rm", "-f", envName) })
	out := anyctr(t, "run", "--rm", "--name", envName, "-e", "ANYCTR_CHECK=ok",
		"-w", "/tmp", testImage, "sh", "-c", "echo $ANYCTR_CHECK; pwd")
	if !strings.Contains(out, "ok") || !strings.Contains(out, "/tmp") {
		t.Errorf("-e/-w did not reach the container:\n%s", out)
	}

	anyctr(t, "stop", name)
	if out := anyctr(t, "ps", "-a"); !strings.Contains(out, name) {
		t.Errorf("a stopped container is missing from `anyctr ps -a`:\n%s", out)
	}
	anyctr(t, "rm", name)
	if out := anyctr(t, "ps", "-a"); strings.Contains(out, name) {
		t.Errorf("container still listed after rm:\n%s", out)
	}
}

// TestLogsAndExitCode covers the two things a CI script depends on.
func TestLogsAndExitCode(t *testing.T) {
	anyctr(t, "pull", testImage)

	name := uniqueName("logs")
	t.Cleanup(func() { tryAnyctr(t, "rm", "-f", name) })
	anyctr(t, "run", "-d", "--name", name, testImage,
		"sh", "-c", "echo line-one; echo line-two; sleep 300")

	// Logs are written asynchronously by every runtime here.
	var logs string
	for i := 0; i < 20; i++ {
		if logs = anyctr(t, "logs", name); strings.Contains(logs, "line-two") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !strings.Contains(logs, "line-one") || !strings.Contains(logs, "line-two") {
		t.Errorf("logs are missing output:\n%s", logs)
	}

	// A container's exit status must become anyctr's exit status.
	failer := uniqueName("exit")
	t.Cleanup(func() { tryAnyctr(t, "rm", "-f", failer) })
	if _, code := tryAnyctr(t, "run", "--rm", "--name", failer, testImage, "sh", "-c", "exit 7"); code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
}

// TestBuildTagAndRemove covers the image half of the gateway.
func TestBuildTagAndRemove(t *testing.T) {
	dir := t.TempDir()
	dockerfile := "FROM " + testImage + "\nRUN echo built-by-anyctr > /marker\n"
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		t.Fatal(err)
	}

	tag := strings.ToLower(uniqueName("img")) + ":test"
	alias := strings.ToLower(uniqueName("alias")) + ":test"
	t.Cleanup(func() {
		tryAnyctr(t, "rmi", "-f", alias)
		tryAnyctr(t, "rmi", "-f", tag)
	})

	anyctr(t, "build", "-t", tag, dir)
	if out := anyctr(t, "images"); !strings.Contains(out, strings.SplitN(tag, ":", 2)[0]) {
		t.Errorf("built image is missing from `anyctr images`:\n%s", out)
	}

	name := uniqueName("built")
	t.Cleanup(func() { tryAnyctr(t, "rm", "-f", name) })
	if out := anyctr(t, "run", "--rm", "--name", name, tag, "cat", "/marker"); !strings.Contains(out, "built-by-anyctr") {
		t.Errorf("the built image did not contain the build output:\n%s", out)
	}

	anyctr(t, "tag", tag, alias)
	if out := anyctr(t, "images"); !strings.Contains(out, strings.SplitN(alias, ":", 2)[0]) {
		t.Errorf("tagged alias is missing from `anyctr images`:\n%s", out)
	}
}

// TestStrictRefusalOnRealRuntime checks that the refusal path exits 125 and runs
// nothing, against the runtime that is actually installed.
func TestStrictRefusalOnRealRuntime(t *testing.T) {
	out, code := tryAnyctr(t, "run", "--cgroupns=host", testImage)
	if code != 125 {
		t.Errorf("an unmodelled flag should exit 125, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, "anyctr raw") {
		t.Errorf("the refusal should point at the escape hatch:\n%s", out)
	}
}

// TestComposeRoutes only runs where Compose exists.
func TestComposeRoutes(t *testing.T) {
	if out, code := tryAnyctr(t, "--explain", "compose", "version"); code != 0 {
		t.Skipf("no Compose on this runtime:\n%s", out)
	}
	dir := t.TempDir()
	compose := "services:\n  hello:\n    image: " + testImage + "\n    command: [\"echo\", \"compose-via-anyctr\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	project := strings.ToLower(uniqueName("compose"))
	t.Cleanup(func() { tryAnyctr(t, "compose", "-f", filepath.Join(dir, "compose.yaml"), "-p", project, "down", "-v") })

	out, code := tryAnyctr(t, "compose", "-f", filepath.Join(dir, "compose.yaml"), "-p", project, "up")
	if code != 0 {
		t.Fatalf("compose up exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "compose-via-anyctr") {
		t.Errorf("compose service output missing:\n%s", out)
	}
}
