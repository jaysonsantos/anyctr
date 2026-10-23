// Package docker adapts the Docker CLI. It is the reference runtime: anyctr's
// command surface is Docker's, so translation here is close to the identity
// function, and the capability map only records the handful of flags anyctr
// models for other runtimes that Docker itself does not have.
package docker

import (
	"github.com/pedrospdc/anyctr/internal/runtime"
	"github.com/pedrospdc/anyctr/internal/runtime/dockerlike"
	"github.com/pedrospdc/anyctr/internal/spec"
)

// Adapter is the docker runtime.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (*Adapter) Name() string        { return "docker" }
func (*Adapter) Binary() string      { return "docker" }
func (*Adapter) Aliases() []string   { return nil }
func (*Adapter) Platforms() []string { return nil }
func (*Adapter) Description() string { return "Docker Engine / Docker Desktop" }

func (*Adapter) ParseVersion(raw string) string { return runtime.ExtractVersion(raw) }

func (*Adapter) Label(version string) string {
	if version == "" {
		return "docker"
	}
	return "docker " + version
}

// noAll is shared by stop/rm/rmi: anyctr accepts --all because most runtimes
// have it, but Docker never grew the flag.
func noAll(cmd string) runtime.Support {
	return runtime.No(
		"The Docker CLI has no --all for `docker "+cmd+"`.",
		"Nearest option: pass the IDs explicitly, e.g.",
		"    anyctr "+cmd+" $(anyctr ps -aq)",
	)
}

// renames covers the flags Docker spells differently from the canonical name.
// `docker stop --time` was renamed to --timeout and no longer appears in help.
var renames = map[string]string{"time": "--timeout"}

func (*Adapter) Caps() runtime.Caps {
	return runtime.Caps{
		"stop.all": noAll("stop"),
		"rm.all":   noAll("rm"),
		"rmi.all":  noAll("rmi"),
	}
}

func (a *Adapter) Translate(op spec.Op, tr *runtime.Tr) (runtime.Invocation, error) {
	if c, ok := op.(*spec.Compose); ok {
		// Compose v2 is a CLI plugin: `docker compose ...`.
		return runtime.Invocation{
			Bin:   tr.Info.Binary,
			Args:  append([]string{"compose"}, c.Args...),
			TTY:   true,
			Stdin: true,
		}, nil
	}
	args, err := dockerlike.Translate(op, tr, renames)
	if err != nil {
		return runtime.Invocation{}, err
	}
	return invocation(tr.Info.Binary, args, op), nil
}

// invocation attaches the terminal expectations that the executor needs.
func invocation(bin string, args []string, op spec.Op) runtime.Invocation {
	tty, stdin := runtime.Terminal(op)
	return runtime.Invocation{Bin: bin, Args: args, TTY: tty, Stdin: stdin}
}
