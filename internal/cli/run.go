package cli

import (
	"github.com/spf13/cobra"

	"github.com/pedrospdc/anyctr/internal/spec"
)

func newRunCmd(a *app) *cobra.Command {
	s := &spec.Run{}
	cmd := &cobra.Command{
		Use:   "run [OPTIONS] IMAGE [COMMAND] [ARG...]",
		Short: "Create and run a new container from an image",
		Args:  requireArgs("run", 1, "run [OPTIONS] IMAGE [COMMAND] [ARG...]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s.Image = args[0]
			s.Cmd = args[1:]
			// Docker treats --entrypoint="" as "clear the image entrypoint", so
			// the flag's presence matters independently of its value.
			s.EntrypointSet = changed(cmd, "entrypoint")
			return a.gateway(s)
		},
	}
	dockerStyle(cmd)
	// `docker run` spends -h on --hostname, so anyctr does too.
	reserveHelpShorthand(cmd)

	f := cmd.Flags()
	f.StringArrayVar(&s.AddHost, "add-host", nil, "Add a custom host-to-IP mapping (host:ip)")
	f.StringArrayVar(&s.CapAdd, "cap-add", nil, "Add a Linux capability")
	f.StringArrayVar(&s.CapDrop, "cap-drop", nil, "Drop a Linux capability")
	f.StringVar(&s.Cidfile, "cidfile", "", "Write the container ID to the file")
	f.StringVar(&s.CPUShares, "cpu-shares", "", "CPU shares (relative weight)")
	f.StringVar(&s.CPUs, "cpus", "", "Number of CPUs")
	f.BoolVarP(&s.Detach, "detach", "d", false, "Run container in background and print container ID")
	f.StringArrayVar(&s.Device, "device", nil, "Add a host device to the container")
	f.StringArrayVar(&s.DNS, "dns", nil, "Set custom DNS servers")
	f.StringArrayVar(&s.DNSOption, "dns-option", nil, "Set DNS options")
	f.StringArrayVar(&s.DNSSearch, "dns-search", nil, "Set custom DNS search domains")
	f.StringVar(&s.Entrypoint, "entrypoint", "", "Overwrite the default ENTRYPOINT of the image")
	f.StringArrayVarP(&s.Env, "env", "e", nil, "Set environment variables")
	f.StringArrayVar(&s.EnvFile, "env-file", nil, "Read in a file of environment variables")
	f.StringVar(&s.GPUs, "gpus", "", "GPU devices to add to the container ('all' to pass all GPUs)")
	f.StringVarP(&s.Hostname, "hostname", "h", "", "Container host name")
	f.BoolVar(&s.Init, "init", false, "Run an init inside the container that forwards signals and reaps processes")
	f.BoolVarP(&s.Interactive, "interactive", "i", false, "Keep STDIN open even if not attached")
	f.StringVar(&s.IPC, "ipc", "", "IPC mode to use")
	f.StringArrayVarP(&s.Label, "label", "l", nil, "Set metadata on a container")
	f.StringArrayVar(&s.LabelFile, "label-file", nil, "Read in a line-delimited file of labels")
	f.StringVar(&s.LogDriver, "log-driver", "", "Logging driver for the container")
	f.StringArrayVar(&s.LogOpt, "log-opt", nil, "Log driver options")
	f.StringVarP(&s.Memory, "memory", "m", "", "Memory limit")
	f.StringVar(&s.MemorySwap, "memory-swap", "", "Swap limit equal to memory plus swap")
	f.StringArrayVar(&s.Mount, "mount", nil, "Attach a filesystem mount to the container")
	f.StringVar(&s.Name, "name", "", "Assign a name to the container")
	f.StringArrayVar(&s.Network, "network", nil, "Connect a container to a network")
	f.StringArrayVar(&s.NetworkAlias, "network-alias", nil, "Add network-scoped alias for the container")
	f.StringVar(&s.PID, "pid", "", "PID namespace to use")
	f.StringVar(&s.Platform, "platform", "", "Set platform if server is multi-platform capable")
	f.BoolVar(&s.Privileged, "privileged", false, "Give extended privileges to this container")
	f.StringArrayVarP(&s.Publish, "publish", "p", nil, "Publish a container's port(s) to the host")
	f.BoolVarP(&s.PublishAll, "publish-all", "P", false, "Publish all exposed ports to random ports")
	f.StringVar(&s.Pull, "pull", "", `Pull image before running ("always", "missing", "never")`)
	f.BoolVar(&s.ReadOnly, "read-only", false, "Mount the container's root filesystem as read only")
	f.StringVar(&s.Restart, "restart", "", "Restart policy to apply when a container exits")
	f.BoolVar(&s.Rm, "rm", false, "Automatically remove the container when it exits")
	f.StringArrayVar(&s.SecurityOpt, "security-opt", nil, "Security Options")
	f.StringVar(&s.ShmSize, "shm-size", "", "Size of /dev/shm")
	f.StringVar(&s.StopSignal, "stop-signal", "", "Signal to stop the container")
	f.StringVar(&s.StopTimeout, "stop-timeout", "", "Timeout (in seconds) to stop a container")
	f.StringArrayVar(&s.Sysctl, "sysctl", nil, "Sysctl options")
	f.StringArrayVar(&s.Tmpfs, "tmpfs", nil, "Mount a tmpfs directory")
	f.BoolVarP(&s.TTY, "tty", "t", false, "Allocate a pseudo-TTY")
	f.StringArrayVar(&s.Ulimit, "ulimit", nil, "Ulimit options")
	f.StringVarP(&s.User, "user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")
	f.StringVar(&s.Userns, "userns", "", "User namespace to use")
	f.StringArrayVarP(&s.Volume, "volume", "v", nil, "Bind mount a volume")
	f.StringVarP(&s.Workdir, "workdir", "w", "", "Working directory inside the container")

	return flagErrors(cmd)
}
