package cli

import (
	"github.com/spf13/cobra"

	"github.com/pedrospdc/anyctr/internal/spec"
)

func newPsCmd(a *app) *cobra.Command {
	s := &spec.Ps{}
	cmd := &cobra.Command{
		Use:     "ps [OPTIONS]",
		Short:   "List containers",
		Aliases: []string{"list"},
		Args:    cobra.NoArgs,
		RunE:    func(*cobra.Command, []string) error { return a.gateway(s) },
	}
	f := cmd.Flags()
	f.BoolVarP(&s.All, "all", "a", false, "Show all containers (default shows just running)")
	f.StringArrayVarP(&s.Filter, "filter", "f", nil, "Filter output based on conditions provided")
	f.StringVar(&s.Format, "format", "", "Format output using a custom template or json")
	f.StringVarP(&s.Last, "last", "n", "", "Show n last created containers (includes all states)")
	f.BoolVarP(&s.Latest, "latest", "l", false, "Show the latest created container")
	f.BoolVar(&s.NoTrunc, "no-trunc", false, "Don't truncate output")
	f.BoolVarP(&s.Quiet, "quiet", "q", false, "Only display container IDs")
	f.BoolVarP(&s.Size, "size", "s", false, "Display total file sizes")
	return flagErrors(cmd)
}

func newLogsCmd(a *app) *cobra.Command {
	s := &spec.Logs{}
	cmd := &cobra.Command{
		Use:   "logs [OPTIONS] CONTAINER",
		Short: "Fetch the logs of a container",
		Args:  requireArgs("logs", 1, "logs [OPTIONS] CONTAINER"),
		RunE: func(_ *cobra.Command, args []string) error {
			s.Container = args[0]
			return a.gateway(s)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&s.Details, "details", false, "Show extra details provided to logs")
	f.BoolVarP(&s.Follow, "follow", "f", false, "Follow log output")
	f.StringVar(&s.Since, "since", "", "Show logs since timestamp or relative duration")
	f.StringVar(&s.Tail, "tail", "", "Number of lines to show from the end of the logs")
	f.BoolVarP(&s.Timestamps, "timestamps", "t", false, "Show timestamps")
	f.StringVar(&s.Until, "until", "", "Show logs before timestamp or relative duration")
	return flagErrors(cmd)
}

func newExecCmd(a *app) *cobra.Command {
	s := &spec.Exec{}
	cmd := &cobra.Command{
		Use:   "exec [OPTIONS] CONTAINER COMMAND [ARG...]",
		Short: "Execute a command in a running container",
		Args:  requireArgs("exec", 2, "exec [OPTIONS] CONTAINER COMMAND [ARG...]"),
		RunE: func(_ *cobra.Command, args []string) error {
			s.Container = args[0]
			s.Cmd = args[1:]
			return a.gateway(s)
		},
	}
	dockerStyle(cmd)
	f := cmd.Flags()
	f.BoolVarP(&s.Detach, "detach", "d", false, "Detached mode: run command in the background")
	f.StringArrayVarP(&s.Env, "env", "e", nil, "Set environment variables")
	f.StringArrayVar(&s.EnvFile, "env-file", nil, "Read in a file of environment variables")
	f.BoolVarP(&s.Interactive, "interactive", "i", false, "Keep STDIN open even if not attached")
	f.BoolVar(&s.Privileged, "privileged", false, "Give extended privileges to the command")
	f.BoolVarP(&s.TTY, "tty", "t", false, "Allocate a pseudo-TTY")
	f.StringVarP(&s.User, "user", "u", "", "Username or UID")
	f.StringVarP(&s.Workdir, "workdir", "w", "", "Working directory inside the container")
	return flagErrors(cmd)
}

func newStopCmd(a *app) *cobra.Command {
	s := &spec.Stop{}
	cmd := &cobra.Command{
		Use:   "stop [OPTIONS] CONTAINER [CONTAINER...]",
		Short: "Stop one or more running containers",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !s.All {
				return requireArgs("stop", 1, "stop [OPTIONS] CONTAINER [CONTAINER...]")(cmd, args)
			}
			s.Containers = args
			return a.gateway(s)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&s.All, "all", "a", false, "Stop all running containers (not a Docker flag)")
	f.StringVarP(&s.Signal, "signal", "s", "", "Signal to send to the container")
	f.StringVarP(&s.Time, "time", "t", "", "Seconds to wait before killing the container")
	return flagErrors(cmd)
}

func newRmCmd(a *app) *cobra.Command {
	s := &spec.Rm{}
	cmd := &cobra.Command{
		Use:   "rm [OPTIONS] CONTAINER [CONTAINER...]",
		Short: "Remove one or more containers",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !s.All {
				return requireArgs("rm", 1, "rm [OPTIONS] CONTAINER [CONTAINER...]")(cmd, args)
			}
			s.Containers = args
			return a.gateway(s)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&s.All, "all", "a", false, "Remove all containers (not a Docker flag)")
	f.BoolVarP(&s.Force, "force", "f", false, "Force the removal of a running container")
	f.BoolVarP(&s.Link, "link", "l", false, "Remove the specified link")
	f.BoolVarP(&s.Volumes, "volumes", "v", false, "Remove anonymous volumes associated with the container")
	return flagErrors(cmd)
}
