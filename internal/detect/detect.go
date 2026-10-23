// Package detect finds which container runtimes are actually installed and picks
// the one a command should target.
//
// Selection precedence, highest first:
//
//	--runtime=<name>        explicit, per invocation
//	ANYCTR_RUNTIME=<name>   explicit, per shell
//	runtime = "<name>"      config file pin
//	priority order          first runtime found on PATH
//
// Nothing here talks to a daemon: availability means "the CLI exists and reports
// a version", which is cheap and works when the engine is stopped.
package detect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/pedrospdc/anyctr/internal/config"
	"github.com/pedrospdc/anyctr/internal/diag"
	"github.com/pedrospdc/anyctr/internal/runtime"
	"github.com/pedrospdc/anyctr/internal/runtime/applecontainer"
	"github.com/pedrospdc/anyctr/internal/runtime/docker"
	"github.com/pedrospdc/anyctr/internal/runtime/podman"
)

// DefaultPriority is the auto-detection order. Docker comes first because its
// convention is the one anyctr speaks, so it is the least surprising default when
// an engineer has several runtimes installed.
var DefaultPriority = []string{"docker", "podman", "container"}

// versionTimeout bounds the `--version` probe. A wedged CLI must not hang the
// gateway.
const versionTimeout = 5 * time.Second

// Adapters returns every runtime anyctr knows how to translate for.
func Adapters() []runtime.Adapter {
	return []runtime.Adapter{
		docker.New(),
		podman.New(),
		applecontainer.New(),
	}
}

// Names lists the known runtime names, in registration order.
func Names() []string {
	adapters := Adapters()
	names := make([]string, 0, len(adapters))
	for _, a := range adapters {
		names = append(names, a.Name())
	}
	return names
}

// Lookup finds an adapter by name.
func Lookup(name string) runtime.Adapter {
	for _, a := range Adapters() {
		if a.Name() == name {
			return a
		}
	}
	// Convenience: `--runtime=apple` and `--runtime=docker-compose` are what
	// engineers actually type.
	switch name {
	case "apple", "apple-container", "applecontainer":
		return applecontainer.New()
	case "docker-compose", "compose":
		return docker.New()
	}
	return nil
}

// Found is the result of probing one runtime on this machine.
type Found struct {
	Adapter   runtime.Adapter
	Path      string // resolved executable path, empty when not installed
	Version   string // as parsed from `--version`, empty when unknown
	Available bool
	Reason    string // why it is unavailable
}

// Info renders the runtime identity used during translation.
func (f Found) Info() runtime.Info {
	bin := f.Path
	if bin == "" {
		bin = f.Adapter.Binary()
	}
	return runtime.Info{
		Name:    f.Adapter.Name(),
		Binary:  bin,
		Version: f.Version,
		Label:   f.Adapter.Label(f.Version),
	}
}

// Probe checks a single runtime: is its CLI on PATH, and what version is it?
func Probe(a runtime.Adapter, cfg *config.Config) Found {
	f := Found{Adapter: a}

	if !supportsPlatform(a) {
		f.Reason = "not available on " + goruntime.GOOS
		return f
	}

	candidates := make([]string, 0, 3)
	if cfg != nil {
		if override := cfg.Binaries[a.Name()]; override != "" {
			candidates = append(candidates, override)
		}
	}
	candidates = append(candidates, a.Binary())
	candidates = append(candidates, a.Aliases()...)

	shadowed := false
	for _, c := range candidates {
		path, err := exec.LookPath(c)
		if err != nil {
			continue
		}
		// `anyctr alias` puts a `docker` on PATH that is anyctr itself. Selecting
		// it would make anyctr exec itself forever, so it is never a runtime.
		if IsSelf(path) {
			shadowed = true
			continue
		}
		f.Path = path
		f.Available = true
		f.Version = probeVersion(path, a)
		return f
	}
	f.Reason = "not found on PATH"
	if shadowed {
		f.Reason = "not installed (the " + a.Binary() + " on PATH is anyctr's own alias)"
	}
	return f
}

// List probes every known runtime, in the configured priority order followed by
// any remaining ones, so `anyctr runtime list` reads like the selection order.
func List(cfg *config.Config) []Found {
	order := priority(cfg)
	seen := map[string]bool{}
	out := make([]Found, 0, len(Adapters()))
	for _, name := range order {
		a := Lookup(name)
		if a == nil || seen[a.Name()] {
			continue
		}
		seen[a.Name()] = true
		out = append(out, Probe(a, cfg))
	}
	for _, a := range Adapters() {
		if seen[a.Name()] {
			continue
		}
		seen[a.Name()] = true
		out = append(out, Probe(a, cfg))
	}
	return out
}

// Request describes how a runtime was asked for.
type Request struct {
	// Name is the explicit selection (flag or environment), if any.
	Name string
	// Explain skips the availability requirement: translating a command for a
	// runtime you do not have installed is a legitimate thing to want, and it is
	// what makes the translation layer testable on any OS.
	Explain bool
}

// Select resolves which runtime a command should target.
func Select(cfg *config.Config, req Request) (runtime.Adapter, runtime.Info, error) {
	name := req.Name
	if name == "" && cfg != nil {
		name = cfg.Runtime
	}

	if name != "" {
		a := Lookup(name)
		if a == nil {
			return nil, runtime.Info{}, &diag.UnknownRuntime{Name: name, Known: Names()}
		}
		f := Probe(a, cfg)
		if !f.Available && !req.Explain {
			hint := ""
			if !supportsPlatform(a) {
				hint = "This runtime only exists on " + strings.Join(a.Platforms(), ", ") + "."
			}
			return nil, runtime.Info{}, &diag.RuntimeUnavailable{
				Name:   a.Name(),
				Binary: a.Binary(),
				Hint:   hint,
			}
		}
		return a, f.Info(), nil
	}

	for _, f := range List(cfg) {
		if f.Available {
			return f.Adapter, f.Info(), nil
		}
	}

	searched := make([]string, 0, len(Adapters()))
	for _, a := range Adapters() {
		searched = append(searched, a.Binary())
	}
	return nil, runtime.Info{}, &diag.NoRuntime{Searched: searched}
}

// IsSelf reports whether a binary on PATH is the running anyctr — directly, via
// a symlink, or through the .cmd shim used on Windows.
func IsSelf(path string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if SameFile(path, exe) {
		return true
	}
	if goruntime.GOOS == "windows" {
		content, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		return strings.Contains(strings.ToLower(string(content)), strings.ToLower(exe))
	}
	return false
}

// SameFile reports whether two paths resolve to the same file.
func SameFile(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

func priority(cfg *config.Config) []string {
	if cfg != nil && len(cfg.Priority) > 0 {
		return cfg.Priority
	}
	return DefaultPriority
}

func supportsPlatform(a runtime.Adapter) bool {
	platforms := a.Platforms()
	if len(platforms) == 0 {
		return true
	}
	for _, p := range platforms {
		if p == goruntime.GOOS {
			return true
		}
	}
	return false
}

// probeVersion asks a CLI for its version, bounded in time because some of them
// are slow to start (a VM manager may spin up state first).
//
// The timeout is enforced with a context rather than a racing goroutine: a
// goroutine left behind after the timeout keeps touching process state — the
// environment among it — long after the probe returned, which is both a leak and
// a data race with anything that writes to the environment afterwards.
func probeVersion(path string, a runtime.Adapter) string {
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return a.ParseVersion(strings.TrimSpace(string(out)))
}
