package config

import (
	"path/filepath"
	"testing"
)

func TestMissingFileIsNotAnError(t *testing.T) {
	// anyctr must be useful before anyone configures it.
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("a missing config must not fail: %v", err)
	}
	if !cfg.Strict {
		t.Error("strict must default to true: silence is the failure mode we are avoiding")
	}
}

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{
		{"runtime", "podman"},
		{"strict", "false"},
		{"priority", "container,podman,docker"},
		{"binaries.docker", "/opt/bin/docker"},
		{"flags.restart", "drop"},
	} {
		if err := cfg.Set(kv[0], kv[1]); err != nil {
			t.Fatalf("Set(%q, %q): %v", kv[0], kv[1], err)
		}
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Runtime != "podman" || reloaded.Strict {
		t.Errorf("scalars did not survive the round trip: %+v", reloaded)
	}
	if len(reloaded.Priority) != 3 || reloaded.Priority[0] != "container" {
		t.Errorf("priority did not survive: %v", reloaded.Priority)
	}
	if reloaded.Binaries["docker"] != "/opt/bin/docker" {
		t.Errorf("binary override did not survive: %v", reloaded.Binaries)
	}
	if reloaded.Flags["restart"] != "drop" {
		t.Errorf("flag policy did not survive: %v", reloaded.Flags)
	}
}

func TestRejectsNonsense(t *testing.T) {
	cfg := Default()
	if err := cfg.Set("strict", "maybe"); err == nil {
		t.Error("strict must be a boolean")
	}
	if err := cfg.Set("flags.restart", "ignore"); err == nil {
		t.Error("flag policy must be error or drop")
	}
	if err := cfg.Set("nonsense", "x"); err == nil {
		t.Error("unknown keys must be rejected, not silently stored")
	}
}

func TestUnsetRestoresDefaults(t *testing.T) {
	cfg := Default()
	if err := cfg.Set("strict", "false"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Set("strict", ""); err != nil {
		t.Fatal(err)
	}
	if !cfg.Strict {
		t.Error("unsetting strict must restore the safe default")
	}
}

func TestPathHonoursEnvironment(t *testing.T) {
	t.Setenv("ANYCTR_CONFIG", "/tmp/explicit.toml")
	if got := Path(); got != "/tmp/explicit.toml" {
		t.Errorf("ANYCTR_CONFIG must win, got %q", got)
	}
}
