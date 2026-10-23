// Package dockerlike translates operations for CLIs that already speak Docker's
// convention — docker itself, and podman, which is deliberately argv-compatible.
//
// Because the spec IR is modelled on Docker's flags, this translation is mostly
// mechanical. The value it adds is that every flag still goes through
// runtime.Tr.Allow, so a runtime in this family can declare its own gaps
// (podman's --gpus, say) and get the same strict errors and warnings as any
// other adapter.
package dockerlike

import (
	"github.com/pedrospdc/anyctr/internal/runtime"
	"github.com/pedrospdc/anyctr/internal/spec"
)

// Translate renders a Docker-convention argv for the given operation.
func Translate(op spec.Op, tr *runtime.Tr) ([]string, error) {
	b := runtime.NewBuilder(tr, true)
	switch s := op.(type) {
	case *spec.Run:
		run(b, s)
	case *spec.Ps:
		ps(b, s)
	case *spec.Images:
		images(b, s)
	case *spec.Build:
		build(b, s)
	case *spec.Pull:
		pull(b, s)
	case *spec.Push:
		push(b, s)
	case *spec.Logs:
		logs(b, s)
	case *spec.Exec:
		exec(b, s)
	case *spec.Stop:
		stop(b, s)
	case *spec.Rm:
		rm(b, s)
	case *spec.Rmi:
		rmi(b, s)
	case *spec.Tag:
		b.Add("tag", s.Source, s.Target)
	default:
		return nil, &runtime.ErrUnsupportedOp{Op: op.Command(), Runtime: tr.Info.Name}
	}
	return b.Args(), tr.Err()
}

func run(b *runtime.Builder, s *spec.Run) {
	b.Add("run")
	b.Rep("add-host", s.AddHost)
	b.Rep("cap-add", s.CapAdd)
	b.Rep("cap-drop", s.CapDrop)
	b.Str("cidfile", s.Cidfile)
	b.Str("cpu-shares", s.CPUShares)
	b.Str("cpus", s.CPUs)
	b.Bool("detach", s.Detach)
	b.Rep("device", s.Device)
	b.Rep("dns", s.DNS)
	b.Rep("dns-option", s.DNSOption)
	b.Rep("dns-search", s.DNSSearch)
	if s.EntrypointSet {
		b.Raw("entrypoint", "--entrypoint="+s.Entrypoint)
	}
	b.Rep("env", s.Env)
	b.Rep("env-file", s.EnvFile)
	b.Str("gpus", s.GPUs)
	b.Str("hostname", s.Hostname)
	b.Bool("init", s.Init)
	b.Bool("interactive", s.Interactive)
	b.Str("ipc", s.IPC)
	b.Rep("label", s.Label)
	b.Rep("label-file", s.LabelFile)
	b.Str("log-driver", s.LogDriver)
	b.Rep("log-opt", s.LogOpt)
	b.Str("memory", s.Memory)
	b.Str("memory-swap", s.MemorySwap)
	b.Rep("mount", s.Mount)
	b.Str("name", s.Name)
	b.Rep("network", s.Network)
	b.Rep("network-alias", s.NetworkAlias)
	b.Str("pid", s.PID)
	b.Str("platform", s.Platform)
	b.Bool("privileged", s.Privileged)
	b.Rep("publish", s.Publish)
	b.Bool("publish-all", s.PublishAll)
	b.Str("pull", s.Pull)
	b.Bool("read-only", s.ReadOnly)
	b.Str("restart", s.Restart)
	b.Bool("rm", s.Rm)
	b.Rep("security-opt", s.SecurityOpt)
	b.Str("shm-size", s.ShmSize)
	b.Str("stop-signal", s.StopSignal)
	b.Str("stop-timeout", s.StopTimeout)
	b.Rep("sysctl", s.Sysctl)
	b.Rep("tmpfs", s.Tmpfs)
	b.Bool("tty", s.TTY)
	b.Rep("ulimit", s.Ulimit)
	b.Str("user", s.User)
	b.Str("userns", s.Userns)
	b.Rep("volume", s.Volume)
	b.Str("workdir", s.Workdir)
	b.Add(s.Image)
	b.Add(s.Cmd...)
}

func ps(b *runtime.Builder, s *spec.Ps) {
	b.Add("ps")
	b.Bool("all", s.All)
	b.Rep("filter", s.Filter)
	b.Str("format", s.Format)
	b.Str("last", s.Last)
	b.Bool("latest", s.Latest)
	b.Bool("no-trunc", s.NoTrunc)
	b.Bool("quiet", s.Quiet)
	b.Bool("size", s.Size)
}

func images(b *runtime.Builder, s *spec.Images) {
	b.Add("images")
	b.Bool("all", s.All)
	b.Bool("digests", s.Digests)
	b.Rep("filter", s.Filter)
	b.Str("format", s.Format)
	b.Bool("no-trunc", s.NoTrunc)
	b.Bool("quiet", s.Quiet)
	if s.Reference != "" {
		b.Add(s.Reference)
	}
}

func build(b *runtime.Builder, s *spec.Build) {
	b.Add("build")
	b.Rep("add-host", s.AddHost)
	b.Rep("build-arg", s.BuildArg)
	b.Rep("cache-from", s.CacheFrom)
	b.Rep("cache-to", s.CacheTo)
	b.Str("file", s.File)
	b.Rep("label", s.Label)
	b.Str("network", s.Network)
	b.Bool("no-cache", s.NoCache)
	b.Str("output", s.Output)
	b.Str("platform", s.Platform)
	b.Str("progress", s.Progress)
	b.Bool("pull", s.Pull)
	b.Bool("quiet", s.Quiet)
	b.Rep("secret", s.Secret)
	b.Rep("ssh", s.SSH)
	b.Rep("tag", s.Tag)
	b.Str("target", s.Target)
	b.Add(s.Context)
}

func pull(b *runtime.Builder, s *spec.Pull) {
	b.Add("pull")
	b.Bool("all-tags", s.AllTags)
	b.Bool("disable-content-trust", s.DisableContentTrust)
	b.Str("platform", s.Platform)
	b.Bool("quiet", s.Quiet)
	b.Add(s.Reference)
}

func push(b *runtime.Builder, s *spec.Push) {
	b.Add("push")
	b.Bool("all-tags", s.AllTags)
	b.Str("platform", s.Platform)
	b.Bool("quiet", s.Quiet)
	b.Add(s.Reference)
}

func logs(b *runtime.Builder, s *spec.Logs) {
	b.Add("logs")
	b.Bool("details", s.Details)
	b.Bool("follow", s.Follow)
	b.Str("since", s.Since)
	b.Str("tail", s.Tail)
	b.Bool("timestamps", s.Timestamps)
	b.Str("until", s.Until)
	b.Add(s.Container)
}

func exec(b *runtime.Builder, s *spec.Exec) {
	b.Add("exec")
	b.Bool("detach", s.Detach)
	b.Rep("env", s.Env)
	b.Rep("env-file", s.EnvFile)
	b.Bool("interactive", s.Interactive)
	b.Bool("privileged", s.Privileged)
	b.Bool("tty", s.TTY)
	b.Str("user", s.User)
	b.Str("workdir", s.Workdir)
	b.Add(s.Container)
	b.Add(s.Cmd...)
}

func stop(b *runtime.Builder, s *spec.Stop) {
	b.Add("stop")
	b.Bool("all", s.All)
	b.Str("signal", s.Signal)
	b.Str("time", s.Time)
	b.Add(s.Containers...)
}

func rm(b *runtime.Builder, s *spec.Rm) {
	b.Add("rm")
	b.Bool("all", s.All)
	b.Bool("force", s.Force)
	b.Bool("link", s.Link)
	b.Bool("volumes", s.Volumes)
	b.Add(s.Containers...)
}

func rmi(b *runtime.Builder, s *spec.Rmi) {
	b.Add("rmi")
	b.Bool("all", s.All)
	b.Bool("force", s.Force)
	b.Bool("no-prune", s.NoPrune)
	b.Add(s.Images...)
}
