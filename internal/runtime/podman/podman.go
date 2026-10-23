// Package podman adapts the Podman CLI.
//
// Podman is deliberately argv-compatible with Docker, so it reuses the dockerlike
// translation wholesale. What it does not share is semantics: rootless by
// default, CDI instead of the nvidia runtime, its own log driver set. Those are
// recorded as divergences rather than silently accepted, because a command that
// "works" on both but means something different is the failure mode this gateway
// exists to prevent.
package podman

import (
	"github.com/pedrospdc/anyctr/internal/runtime"
	"github.com/pedrospdc/anyctr/internal/runtime/dockerlike"
	"github.com/pedrospdc/anyctr/internal/spec"
)

// Adapter is the podman runtime.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (*Adapter) Name() string        { return "podman" }
func (*Adapter) Binary() string      { return "podman" }
func (*Adapter) Aliases() []string   { return nil }
func (*Adapter) Platforms() []string { return nil }
func (*Adapter) Description() string { return "Podman CLI (daemonless, rootless by default)" }

func (*Adapter) ParseVersion(raw string) string { return runtime.ExtractVersion(raw) }

func (*Adapter) Label(version string) string {
	if version == "" {
		return "podman"
	}
	return "podman " + version
}

func (*Adapter) Caps() runtime.Caps {
	return runtime.Caps{
		"run.gpus": runtime.Div(
			"Podman routes GPUs through CDI; this needs a configured CDI spec " +
				"(e.g. nvidia-ctk cdi generate) rather than a container runtime hook."),
		"run.userns": runtime.Div(
			"Podman's user namespace values differ from Docker's (keep-id, nomap, " +
				"auto), and rootless Podman already runs in a user namespace."),
		"run.log-driver": runtime.Div(
			"Podman's driver set is k8s-file/journald/none/passthrough; " +
				"json-file is an alias for k8s-file."),
		"run.privileged": runtime.Div(
			"Under rootless Podman, --privileged cannot grant more than the " +
				"invoking user already has."),
	}
}

func (a *Adapter) Translate(op spec.Op, tr *runtime.Tr) (runtime.Invocation, error) {
	tty, stdin := runtime.Terminal(op)
	if c, ok := op.(*spec.Compose); ok {
		// `podman compose` is itself a shim that drives an external provider
		// (podman-compose or docker-compose). Detection of the provider is
		// podman's job, not ours.
		return runtime.Invocation{
			Bin:   tr.Info.Binary,
			Args:  append([]string{"compose"}, c.Args...),
			TTY:   tty,
			Stdin: stdin,
		}, nil
	}
	args, err := dockerlike.Translate(op, tr)
	if err != nil {
		return runtime.Invocation{}, err
	}
	return runtime.Invocation{Bin: tr.Info.Binary, Args: args, TTY: tty, Stdin: stdin}, nil
}
