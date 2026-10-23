package detect

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/pedrospdc/anyctr/internal/config"
	"github.com/pedrospdc/anyctr/internal/diag"
)

// fakeRuntime puts an executable named like a runtime CLI on PATH, so detection
// can be tested without installing anything.
func fakeRuntime(t *testing.T, name string) string {
	t.Helper()
	if goruntime.GOOS == "windows" {
		t.Skip("shell stubs are not executable on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\necho '" + name + " version 9.9.9'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

func TestProbeFindsAndVersionsARuntime(t *testing.T) {
	want := fakeRuntime(t, "podman")
	found := Probe(Lookup("podman"), config.Default())
	if !found.Available {
		t.Fatalf("podman should be available: %s", found.Reason)
	}
	if found.Path != want {
		t.Errorf("path = %q, want %q", found.Path, want)
	}
	if found.Version != "9.9.9" {
		t.Errorf("version = %q, want 9.9.9", found.Version)
	}
}

func TestAutoDetectionFollowsPriority(t *testing.T) {
	fakeRuntime(t, "podman")
	adapter, info, err := Select(config.Default(), Request{})
	if err != nil {
		t.Fatalf("auto-detection failed: %v", err)
	}
	if adapter.Name() != "podman" {
		t.Errorf("selected %q, want podman (the only one installed)", adapter.Name())
	}
	if info.Label != "podman 9.9.9" {
		t.Errorf("label = %q", info.Label)
	}
}

func TestConfigPinBeatsAutoDetection(t *testing.T) {
	fakeRuntime(t, "podman")
	cfg := config.Default()
	cfg.Runtime = "docker"

	// A pinned runtime that is not installed is an error, not a silent fallback:
	// falling back would run the engineer's command on a different engine.
	_, _, err := Select(cfg, Request{})
	if err == nil {
		t.Fatal("expected an error for a pinned but missing runtime")
	}
	var unavailable *diag.RuntimeUnavailable
	if !asError(err, &unavailable) {
		t.Fatalf("expected *diag.RuntimeUnavailable, got %T", err)
	}
}

func TestExplainDoesNotRequireInstallation(t *testing.T) {
	fakeRuntime(t, "podman")
	// Translating for a runtime you do not have is legitimate — and it is what
	// makes the translation layer testable on any OS.
	_, info, err := Select(config.Default(), Request{Name: "docker", Explain: true})
	if err != nil {
		t.Fatalf("--explain must not require the runtime: %v", err)
	}
	if info.Binary != "docker" {
		t.Errorf("binary = %q, want the bare name", info.Binary)
	}
}

func TestBinaryOverride(t *testing.T) {
	path := fakeRuntime(t, "my-docker")
	cfg := config.Default()
	cfg.Binaries = map[string]string{"docker": path}

	found := Probe(Lookup("docker"), cfg)
	if !found.Available || found.Path != path {
		t.Errorf("binary override ignored: %+v", found)
	}
}

func TestNoRuntimeFound(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("PATH manipulation differs on Windows")
	}
	t.Setenv("PATH", t.TempDir())
	_, _, err := Select(config.Default(), Request{})
	var none *diag.NoRuntime
	if !asError(err, &none) {
		t.Fatalf("expected *diag.NoRuntime, got %T: %v", err, err)
	}
}

func TestUnknownRuntimeName(t *testing.T) {
	_, _, err := Select(config.Default(), Request{Name: "containerd"})
	var unknown *diag.UnknownRuntime
	if !asError(err, &unknown) {
		t.Fatalf("expected *diag.UnknownRuntime, got %T", err)
	}
}

func TestFriendlyAliases(t *testing.T) {
	// Engineers type what they call the thing, not anyctr's internal name.
	for _, alias := range []string{"apple", "apple-container"} {
		if a := Lookup(alias); a == nil || a.Name() != "container" {
			t.Errorf("alias %q should resolve to Apple's container", alias)
		}
	}
	if a := Lookup("docker-compose"); a == nil || a.Name() != "docker" {
		t.Error("docker-compose should resolve to the docker runtime")
	}
}

func TestApplePlatformGate(t *testing.T) {
	found := Probe(Lookup("container"), config.Default())
	if goruntime.GOOS != "darwin" && found.Available {
		t.Error("Apple's container must never be selected off macOS")
	}
}

// TestAliasIsNeverSelectedAsARuntime is a regression test for a fork bomb.
//
// `anyctr alias` installs a `docker` command that is anyctr itself. Detection
// prefers docker, so without this guard anyctr selected its own alias as the
// runtime and executed itself until the process table filled up.
func TestAliasIsNeverSelectedAsARuntime(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(self, filepath.Join(dir, "docker")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	found := Probe(Lookup("docker"), config.Default())
	if found.Available {
		t.Fatalf("anyctr's own alias must never count as a runtime (path %q)", found.Path)
	}
	if !strings.Contains(found.Reason, "alias") {
		t.Errorf("the reason should explain the alias, got %q", found.Reason)
	}

	// With only the alias on PATH, auto-detection must report no runtime rather
	// than picking the thing that loops.
	if _, _, err := Select(config.Default(), Request{}); err == nil {
		t.Fatal("expected no runtime to be found")
	}
}

// TestIsSelf covers the identity check behind that guard.
func TestIsSelf(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !IsSelf(self) {
		t.Error("the running binary is itself")
	}
	link := filepath.Join(t.TempDir(), "docker")
	if err := os.Symlink(self, link); err != nil {
		t.Fatal(err)
	}
	if !IsSelf(link) {
		t.Error("a symlink to the running binary is itself")
	}
	other := filepath.Join(t.TempDir(), "docker")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if IsSelf(other) {
		t.Error("an unrelated binary is not us")
	}
}
