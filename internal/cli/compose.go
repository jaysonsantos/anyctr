package cli

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/pedrospdc/anyctr/internal/runtime"
	"github.com/pedrospdc/anyctr/internal/spec"
)

// newComposeCmd routes Compose commands to whichever implementation the selected
// runtime ships. anyctr does not interpret compose files yet: it picks the right
// provider and forwards the arguments untouched, which is why flag parsing is
// disabled here.
func newComposeCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compose [OPTIONS] COMMAND [ARG...]",
		Short: "Run Compose, on whichever runtime provides it",
		Long: `Run Compose through the selected runtime.

anyctr does not parse compose files: arguments are forwarded verbatim to the
runtime's own Compose implementation (` + "`docker compose`" + `, ` + "`podman compose`" + `,
or the standalone docker-compose binary). Runtimes without Compose report that
plainly rather than half-working.`,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			args = a.consumeGlobals(args)
			if len(args) == 0 {
				return cmd.Help()
			}
			return a.compose(args)
		},
	}
	return cmd
}

func (a *app) compose(args []string) error {
	adapter, info, err := a.selectRuntime()
	if err != nil {
		return err
	}

	tr := runtime.NewTr(info, adapter.Caps(), a.policy(), "compose")
	inv, err := adapter.Translate(&spec.Compose{Args: args}, tr)
	a.printWarnings(tr.Warnings())
	if err != nil {
		return err
	}

	// Apple's container has no Compose; anyctr routes to an external provider,
	// which has to actually be installed before we run it.
	if info.Name == "container" && !a.explain {
		if _, lookErr := exec.LookPath(inv.Bin); lookErr != nil {
			return fmt.Errorf("Apple's container ships no Compose implementation\n\n"+
				"  anyctr routes `compose` to %s, which is not installed:\n"+
				"    brew install pedrospdc/tap/%s\n\n"+
				"  Or select a runtime that has Compose built in:\n"+
				"    anyctr --runtime=docker compose %s", inv.Bin, inv.Bin, args[0])
		}
	}

	// Compose v2 is a CLI plugin, and plenty of machines still only have the
	// standalone binary. Fall back to it rather than failing on a technicality.
	if info.Name == "docker" && !a.explain && !hasComposePlugin(info.Binary) {
		standalone, lookErr := exec.LookPath("docker-compose")
		if lookErr != nil {
			return fmt.Errorf("docker is installed but Compose is not\n\n"+
				"  Neither the `docker compose` plugin nor a standalone `docker-compose`\n"+
				"  binary was found.\n\n"+
				"  Install Docker Compose, or use a runtime that provides it:\n"+
				"    anyctr --runtime=podman compose %s", args[0])
		}
		fmt.Fprintf(a.err, "anyctr: warning: the docker compose plugin was not found; using %s\n", standalone)
		inv = runtime.Invocation{Bin: standalone, Args: args, TTY: true, Stdin: true}
	}

	return a.dispatch(info, inv)
}

// hasComposePlugin reports whether `docker compose` exists. The probe is a real
// invocation because plugin discovery depends on Docker's own config, not on a
// path anyctr could guess.
func hasComposePlugin(dockerBin string) bool {
	cmd := exec.Command(dockerBin, "compose", "version")
	return cmd.Run() == nil
}
