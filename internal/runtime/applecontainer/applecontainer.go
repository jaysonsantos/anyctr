// Package applecontainer adapts Apple's `container` CLI (github.com/apple/container).
//
// This is where a gateway earns its keep. Apple's CLI is Docker-shaped but not
// Docker-compatible: it runs one lightweight VM per container, so whole families
// of Docker flags (--restart, --privileged, --device, host networking) have no
// meaning, images live under `container image ...`, `ps` is `list`, `rm` is
// `delete`, and --format is an enum rather than a Go template.
//
// Every mapping below was taken from `container --help` output of 1.2.2 rather
// than inferred, and every gap is declared in Caps so engineers get a precise
// error instead of a surprise.
package applecontainer

import (
	"strconv"
	"strings"

	"github.com/pedrospdc/anyctr/internal/runtime"
	"github.com/pedrospdc/anyctr/internal/spec"
)

// Adapter is Apple's container runtime.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (*Adapter) Name() string        { return "container" }
func (*Adapter) Binary() string      { return "container" }
func (*Adapter) Aliases() []string   { return nil }
func (*Adapter) Platforms() []string { return []string{"darwin"} }
func (*Adapter) Description() string { return "Apple silicon, one VM per container" }

func (*Adapter) ParseVersion(raw string) string { return runtime.ExtractVersion(raw) }

func (*Adapter) Label(version string) string {
	if version == "" {
		return "apple/container"
	}
	return "apple/container " + version
}

// ComposeProvider is the external Compose implementation anyctr routes to for
// Apple's container, which has none of its own.
const ComposeProvider = "container-compose"

// ComposeBinary, ComposeInstall and ComposeSource implement
// runtime.ExternalComposer, so the CLI can report the provider in
// `anyctr runtime list` and offer to install it when Compose is first used.
func (*Adapter) ComposeBinary() string { return ComposeProvider }

func (*Adapter) ComposeInstall() []string {
	return []string{"brew", "install", "pedrospdc/tap/" + ComposeProvider}
}

func (*Adapter) ComposeSource() string {
	return "https://github.com/pedrospdc/apple-container-compose"
}

// vmPerContainer is the reason behind most of the gaps below, so it is written
// once.
const vmPerContainer = "Apple's container runs each container in its own lightweight VM, " +
	"and its CLI exposes no equivalent."

func (*Adapter) Caps() runtime.Caps {
	return runtime.Caps{
		// --- run: no equivalent ---
		"run.restart": runtime.No(
			"Apple's container has no restart policy.",
			"Nearest option: supervise the container yourself (launchd, a wrapper script),",
			"or select a runtime that has one:",
			"    anyctr --runtime=podman run --restart=always ...",
		),
		"run.privileged": runtime.No(
			"There is no --privileged: "+vmPerContainer,
			"Nearest option: grant only what you need with --cap-add.",
		),
		"run.device": runtime.No(
			"Host devices cannot be passed into the guest VM.",
			"Nearest option: --virtualization exposes nested virtualization only,",
			"reachable today via: anyctr raw run --virtualization ...",
		),
		"run.gpus": runtime.No("GPU passthrough is not exposed by Apple's container CLI."),
		"run.add-host": runtime.No(
			"There is no --add-host; the guest's /etc/hosts is managed by the runtime.",
			"Nearest option: --dns/--dns-search, or reach other containers by their",
			"container name on a shared --network.",
		),
		"run.hostname": runtime.No(
			"The guest hostname is derived from the container name.",
			"Nearest option: --name sets both the ID and the DNS name.",
		),
		"run.publish-all": runtime.No(
			"There is no -P/--publish-all; Apple's container does not read EXPOSE.",
			"Nearest option: publish each port explicitly with -p.",
		),
		"run.pull": runtime.No(
			"`run` has no --pull policy; a missing image is pulled, a present one is reused.",
			"Nearest option: force a refresh first with `anyctr pull <image>`.",
		),
		"run.sysctl":       runtime.No("Guest sysctls are not settable from the CLI."),
		"run.security-opt": runtime.No("There is no --security-opt: " + vmPerContainer),
		"run.pid":          runtime.No("PID namespaces cannot be shared: " + vmPerContainer),
		"run.ipc":          runtime.No("IPC namespaces cannot be shared: " + vmPerContainer),
		"run.userns":       runtime.No("There is no --userns: " + vmPerContainer),
		"run.log-driver": runtime.No(
			"Logging is fixed; there are no pluggable log drivers.",
			"Nearest option: read logs with `anyctr logs`, or `container logs --boot`",
			"for the VM boot log.",
		),
		"run.log-opt":     runtime.No("Logging is fixed; there are no log driver options."),
		"run.memory-swap": runtime.No("The guest VM has no swap to size."),
		"run.cpu-shares":  runtime.No("There is no relative CPU weighting; use --cpus."),
		"run.network-alias": runtime.No(
			"Network aliases are not supported.",
			"Nearest option: containers resolve each other by --name on a shared network.",
		),
		"run.label-file": runtime.No(
			"There is no --label-file.",
			"Nearest option: pass the labels with repeated --label flags.",
		),
		"run.stop-signal":  runtime.No("The stop signal is chosen at stop time.", "Nearest option: anyctr stop --signal=<sig> <container>"),
		"run.stop-timeout": runtime.No("The stop timeout is chosen at stop time.", "Nearest option: anyctr stop --time=<seconds> <container>"),

		// --- run: supported, with a narrower value domain ---
		"run.cpus": runtime.Ltd(
			"whole CPUs only — Apple's --cpus is a CPU count, not Docker's fractional quota"),
		"run.network": runtime.Ltd(
			"named networks only — there is no host, none, or container: network mode"),
		"run.mount": runtime.Ltd(
			"type=,source=,target=,readonly only — no volume-driver or tmpfs options"),
		"run.tmpfs": runtime.Ltd("mount path only — no size or mode options"),

		// --- ps / images ---
		"ps.format":     runtime.Ltd("json|table|yaml|toml only — Go templates are not supported"),
		"images.format": runtime.Ltd("json|table|yaml|toml only — Go templates are not supported"),
		"ps.filter": runtime.No(
			"`container list` has no server-side filtering.",
			"Nearest option: anyctr ps --format json | jq '...'",
		),
		"ps.last":     runtime.No("There is no -n/--last."),
		"ps.latest":   runtime.No("There is no -l/--latest."),
		"ps.size":     runtime.No("Container sizes are not reported."),
		"ps.no-trunc": runtime.No("IDs are already printed in full."),
		"images.filter": runtime.No(
			"`container image list` has no filtering.",
			"Nearest option: anyctr images --format json | jq '...'",
		),
		"images.all":      runtime.No("There are no intermediate images to reveal."),
		"images.digests":  runtime.No("Digests are not listed.", "Nearest option: anyctr images --format json"),
		"images.no-trunc": runtime.No("Image references are already printed in full."),
		"images.quiet": runtime.Div(
			"Apple's -q prints image references, where Docker's prints image IDs"),

		// --- build ---
		"build.tag":      runtime.Ltd("a single -t/--tag per build"),
		"build.output":   runtime.Ltd("type=oci|tar|local[,dest=] only"),
		"build.progress": runtime.Ltd("auto|plain|tty only"),
		"build.ssh":      runtime.Ltd("--ssh default only"),
		"build.secret":   runtime.Ltd("id=<key>[,env=<VAR>|,src=<path>] only"),
		"build.cache-from": runtime.No(
			"The builder does not import external cache.",
			"Nearest option: keep the builder warm (`container builder start`).",
		),
		"build.cache-to": runtime.No("The builder does not export cache."),
		"build.network":  runtime.No("The build network mode is not selectable."),
		"build.add-host": runtime.No("There is no --add-host at build time."),

		// --- pull / push ---
		"pull.all-tags": runtime.No("Pulling every tag of a repository is not supported.",
			"Nearest option: pull each tag explicitly."),
		"push.all-tags": runtime.No("Pushing every tag of a repository is not supported."),
		"pull.disable-content-trust": runtime.No(
			"There is no content trust toggle; Apple's container does not implement DCT."),
		"pull.quiet": runtime.Div("mapped to --progress none"),
		"push.quiet": runtime.Div("mapped to --progress none"),

		// --- logs ---
		"logs.since":      runtime.No("Time-based log filtering is not supported.", "Nearest option: --tail <n>"),
		"logs.until":      runtime.No("Time-based log filtering is not supported."),
		"logs.timestamps": runtime.No("Log timestamps are not added by the runtime."),
		"logs.details":    runtime.No("There are no extra log attributes to show."),

		// --- exec / rm / rmi ---
		"exec.privileged": runtime.No("There is no --privileged: " + vmPerContainer),
		"rm.volumes": runtime.No(
			"Volumes are not removed with the container.",
			"Nearest option: remove them explicitly with `container volume delete`.",
		),
		"rm.link": runtime.No("Legacy container links do not exist."),
		"rmi.force": runtime.Div(
			"Apple's --force ignores images that are not found, where Docker's forces " +
				"removal of an image still in use"),
		"rmi.no-prune": runtime.No("Untagged parents are not tracked separately."),
	}
}

func (a *Adapter) Translate(op spec.Op, tr *runtime.Tr) (runtime.Invocation, error) {
	// Swift ArgumentParser expects `--flag value`, not `--flag=value`.
	b := runtime.NewBuilder(tr, false)

	switch s := op.(type) {
	case *spec.Run:
		a.run(b, tr, s)
	case *spec.Ps:
		a.ps(b, tr, s)
	case *spec.Images:
		a.images(b, tr, s)
	case *spec.Build:
		a.build(b, tr, s)
	case *spec.Pull:
		a.pull(b, tr, s)
	case *spec.Push:
		a.push(b, tr, s)
	case *spec.Logs:
		a.logs(b, s)
	case *spec.Exec:
		a.exec(b, s)
	case *spec.Stop:
		a.stop(b, s)
	case *spec.Rm:
		a.rm(b, s)
	case *spec.Rmi:
		a.rmi(b, s)
	case *spec.Tag:
		b.Add("image", "tag", s.Source, s.Target)
	case *spec.Compose:
		// Apple ships no Compose of its own, so anyctr routes to the external
		// provider the same way podman's shim does. Availability is checked by
		// the caller, not here, so --explain still works without it installed.
		return runtime.Invocation{
			Bin:   ComposeProvider,
			Args:  s.Args,
			TTY:   true,
			Stdin: true,
		}, nil
	default:
		return runtime.Invocation{}, &runtime.ErrUnsupportedOp{Op: op.Command(), Runtime: a.Name()}
	}

	if err := tr.Err(); err != nil {
		return runtime.Invocation{}, err
	}
	tty, stdin := runtime.Terminal(op)
	return runtime.Invocation{Bin: tr.Info.Binary, Args: b.Args(), TTY: tty, Stdin: stdin}, nil
}

func (a *Adapter) run(b *runtime.Builder, tr *runtime.Tr, s *spec.Run) {
	b.Add("run")
	b.Rep("cap-add", s.CapAdd)
	b.Rep("cap-drop", s.CapDrop)
	b.Str("cidfile", s.Cidfile)
	a.cpus(b, tr, s.CPUs)
	b.Bool("detach", s.Detach)
	b.Rep("dns", s.DNS)
	b.Rep("dns-option", s.DNSOption)
	b.Rep("dns-search", s.DNSSearch)
	if s.EntrypointSet {
		b.Raw("entrypoint", "--entrypoint", s.Entrypoint)
	}
	b.Rep("env", s.Env)
	b.Rep("env-file", s.EnvFile)
	b.Bool("init", s.Init)
	b.Bool("interactive", s.Interactive)
	b.Rep("label", s.Label)
	b.Str("memory", s.Memory)
	b.Rep("mount", s.Mount)
	b.Str("name", s.Name)
	a.network(b, tr, s.Network)
	b.Str("platform", s.Platform)
	b.Rep("publish", s.Publish)
	b.Bool("read-only", s.ReadOnly)
	b.Bool("rm", s.Rm)
	b.Str("shm-size", s.ShmSize)
	b.Rep("tmpfs", s.Tmpfs)
	b.Bool("tty", s.TTY)
	b.Rep("ulimit", s.Ulimit)
	b.Str("user", s.User)
	b.Rep("volume", s.Volume)
	b.Str("workdir", s.Workdir)

	// Flags with no equivalent: consulted so they fail (or warn under --relax)
	// with a precise message instead of vanishing.
	b.Rep("add-host", s.AddHost)
	b.Str("cpu-shares", s.CPUShares)
	b.Rep("device", s.Device)
	b.Str("gpus", s.GPUs)
	b.Str("hostname", s.Hostname)
	b.Str("ipc", s.IPC)
	b.Rep("label-file", s.LabelFile)
	b.Str("log-driver", s.LogDriver)
	b.Rep("log-opt", s.LogOpt)
	b.Str("memory-swap", s.MemorySwap)
	b.Rep("network-alias", s.NetworkAlias)
	b.Str("pid", s.PID)
	b.Bool("privileged", s.Privileged)
	b.Bool("publish-all", s.PublishAll)
	b.Str("pull", s.Pull)
	b.Str("restart", s.Restart)
	b.Rep("security-opt", s.SecurityOpt)
	b.Str("stop-signal", s.StopSignal)
	b.Str("stop-timeout", s.StopTimeout)
	b.Rep("sysctl", s.Sysctl)
	b.Str("userns", s.Userns)

	b.Add(s.Image)
	b.Add(s.Cmd...)
}

// cpus bridges Docker's fractional CPU quota and Apple's whole-CPU count. A
// fractional value is rejected rather than rounded: silently giving a container
// twice the CPU it asked for is exactly the kind of drift this gateway prevents.
func (a *Adapter) cpus(b *runtime.Builder, tr *runtime.Tr, value string) {
	if value == "" {
		return
	}
	if _, err := strconv.ParseUint(value, 10, 32); err != nil {
		tr.Reject("cpus",
			"Apple's --cpus is a whole CPU count, not Docker's fractional quota, "+
				"so --cpus="+value+" cannot be translated faithfully.",
			"Nearest option: round it yourself, e.g. --cpus="+roundUp(value)+",",
			"or select a runtime with quota support:",
			"    anyctr --runtime=podman run --cpus="+value+" ...",
		)
		return
	}
	b.Str("cpus", value)
}

func roundUp(value string) string {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil || f <= 1 {
		return "1"
	}
	n := int(f)
	if float64(n) < f {
		n++
	}
	return strconv.Itoa(n)
}

// network rejects Docker's magic network modes, which describe namespace sharing
// that a VM-per-container runtime cannot offer.
func (a *Adapter) network(b *runtime.Builder, tr *runtime.Tr, networks []string) {
	for _, n := range networks {
		name := strings.SplitN(n, ",", 2)[0]
		switch {
		case name == "host":
			tr.Reject("network",
				"There is no host network mode: "+vmPerContainer,
				"Nearest option: publish the ports you need with -p.",
			)
			return
		case name == "none":
			tr.Reject("network",
				"There is no `none` network mode.",
				"Nearest option: --no-dns limits name resolution, reachable via anyctr raw.",
			)
			return
		case strings.HasPrefix(name, "container:"):
			tr.Reject("network",
				"Network namespaces cannot be shared between containers: "+vmPerContainer,
				"Nearest option: put both containers on the same named network.",
			)
			return
		}
	}
	b.Rep("network", networks)
}

func (a *Adapter) ps(b *runtime.Builder, tr *runtime.Tr, s *spec.Ps) {
	b.Add("list")
	b.Bool("all", s.All)
	a.format(b, tr, s.Format)
	b.Bool("quiet", s.Quiet)

	b.Rep("filter", s.Filter)
	b.Str("last", s.Last)
	b.Bool("latest", s.Latest)
	b.Bool("no-trunc", s.NoTrunc)
	b.Bool("size", s.Size)
}

func (a *Adapter) images(b *runtime.Builder, tr *runtime.Tr, s *spec.Images) {
	b.Add("image", "list")
	a.format(b, tr, s.Format)
	b.Bool("quiet", s.Quiet)

	b.Bool("all", s.All)
	b.Bool("digests", s.Digests)
	b.Rep("filter", s.Filter)
	b.Bool("no-trunc", s.NoTrunc)

	if s.Reference != "" {
		tr.RejectArg("REPOSITORY[:TAG]",
			"`container image list` takes no reference filter.",
			"Nearest option: anyctr images --format json | jq 'map(select(...))'",
		)
	}
}

var appleFormats = map[string]bool{"json": true, "table": true, "yaml": true, "toml": true}

// format validates against Apple's --format enum. Docker's Go templates are the
// common case in scripts, so the error names the escape hatch explicitly.
func (a *Adapter) format(b *runtime.Builder, tr *runtime.Tr, value string) {
	if value == "" {
		return
	}
	normalized := strings.ToLower(value)
	if !appleFormats[normalized] {
		tr.Reject("format",
			"Apple's --format accepts json|table|yaml|toml, not Go templates like "+
				strconv.Quote(value)+".",
			"Nearest option: anyctr "+tr.Command()+" --format json, then post-process (jq).",
		)
		return
	}
	b.Str("format", normalized)
}

func (a *Adapter) build(b *runtime.Builder, tr *runtime.Tr, s *spec.Build) {
	b.Add("build")
	b.Rep("build-arg", s.BuildArg)
	b.Str("file", s.File)
	b.Rep("label", s.Label)
	b.Bool("no-cache", s.NoCache)
	b.Str("output", s.Output)
	b.Str("platform", s.Platform)
	b.Str("progress", s.Progress)
	b.Bool("pull", s.Pull)
	b.Bool("quiet", s.Quiet)
	b.Rep("secret", s.Secret)
	b.Rep("ssh", s.SSH)
	b.Str("target", s.Target)

	if len(s.Tag) > 1 {
		tr.Reject("tag",
			"Apple's container accepts a single -t/--tag per build.",
			"Nearest option: build once, then add references:",
			"    anyctr tag "+s.Tag[0]+" "+s.Tag[1],
		)
	} else {
		b.Rep("tag", s.Tag)
	}

	b.Rep("add-host", s.AddHost)
	b.Rep("cache-from", s.CacheFrom)
	b.Rep("cache-to", s.CacheTo)
	b.Str("network", s.Network)

	b.Add(s.Context)
}

func (a *Adapter) pull(b *runtime.Builder, tr *runtime.Tr, s *spec.Pull) {
	b.Add("image", "pull")
	b.Str("platform", s.Platform)
	if s.Quiet && tr.Allow("quiet") {
		b.Add("--progress", "none")
	}
	b.Bool("all-tags", s.AllTags)
	b.Bool("disable-content-trust", s.DisableContentTrust)
	b.Add(s.Reference)
}

func (a *Adapter) push(b *runtime.Builder, tr *runtime.Tr, s *spec.Push) {
	b.Add("image", "push")
	b.Str("platform", s.Platform)
	if s.Quiet && tr.Allow("quiet") {
		b.Add("--progress", "none")
	}
	b.Bool("all-tags", s.AllTags)
	b.Add(s.Reference)
}

func (a *Adapter) logs(b *runtime.Builder, s *spec.Logs) {
	b.Add("logs")
	b.Bool("follow", s.Follow)
	// Docker's --tail all is the default here, so it needs no flag.
	if s.Tail != "" && s.Tail != "all" {
		b.StrAs("tail", "-n", s.Tail)
	}
	b.Bool("details", s.Details)
	b.Str("since", s.Since)
	b.Bool("timestamps", s.Timestamps)
	b.Str("until", s.Until)
	b.Add(s.Container)
}

func (a *Adapter) exec(b *runtime.Builder, s *spec.Exec) {
	b.Add("exec")
	b.Bool("detach", s.Detach)
	b.Rep("env", s.Env)
	b.Rep("env-file", s.EnvFile)
	b.Bool("interactive", s.Interactive)
	b.Bool("tty", s.TTY)
	b.Str("user", s.User)
	b.Str("workdir", s.Workdir)
	b.Bool("privileged", s.Privileged)
	b.Add(s.Container)
	b.Add(s.Cmd...)
}

func (a *Adapter) stop(b *runtime.Builder, s *spec.Stop) {
	b.Add("stop")
	b.Bool("all", s.All)
	b.Str("signal", s.Signal)
	b.Str("time", s.Time)
	b.Add(s.Containers...)
}

func (a *Adapter) rm(b *runtime.Builder, s *spec.Rm) {
	b.Add("delete")
	b.Bool("all", s.All)
	b.Bool("force", s.Force)
	b.Bool("link", s.Link)
	b.Bool("volumes", s.Volumes)
	b.Add(s.Containers...)
}

func (a *Adapter) rmi(b *runtime.Builder, s *spec.Rmi) {
	b.Add("image", "delete")
	b.Bool("all", s.All)
	b.Bool("force", s.Force)
	b.Bool("no-prune", s.NoPrune)
	b.Add(s.Images...)
}
