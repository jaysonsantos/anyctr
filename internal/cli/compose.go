package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pedrospdc/anyctr/internal/runtime"
	"github.com/pedrospdc/anyctr/internal/spec"
)

// newComposeCmd routes Compose commands to whichever implementation the selected
// runtime provides. anyctr does not interpret compose files yet: it picks the
// right provider and forwards the arguments untouched, which is why flag parsing
// is disabled here.
func newComposeCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compose [OPTIONS] COMMAND [ARG...]",
		Short: "Run Compose, on whichever runtime provides it",
		Long: `Run Compose through the selected runtime.

anyctr does not parse compose files: arguments are forwarded verbatim to the
runtime's Compose provider — ` + "`docker compose`" + `, a standalone docker-compose,
` + "`podman compose`" + `, or container-compose for Apple's container, which ships
none of its own. When that provider is missing, anyctr offers to install it.`,
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

	// A runtime without its own Compose needs a provider on PATH before anything
	// can run. Checking here, rather than letting exec fail, is what lets anyctr
	// name the provider and offer to install it.
	if !a.explain {
		if err := a.ensureComposeProvider(adapter, args); err != nil {
			return err
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

// ensureComposeProvider checks that an external Compose provider is installed,
// offering to install it when anyctr knows how and there is a human to ask.
func (a *app) ensureComposeProvider(adapter runtime.Adapter, args []string) error {
	ext, ok := adapter.(runtime.ExternalComposer)
	if !ok {
		return nil
	}
	bin := ext.ComposeBinary()
	if _, err := exec.LookPath(bin); err == nil {
		return nil
	}

	install := ext.ComposeInstall()
	installable := len(install) > 0
	if installable {
		if _, err := exec.LookPath(install[0]); err != nil {
			installable = false // no installer on this machine; just explain
		}
	}

	if installable && interactive() {
		fmt.Fprintf(a.err, "%s\n\n", composeMissingMessage(adapter.Name(), ext, args))
		if a.confirm(fmt.Sprintf("Install it now with `%s`?", strings.Join(install, " "))) {
			fmt.Fprintln(a.err)
			if err := runInstaller(install); err != nil {
				return fmt.Errorf("installing %s failed: %w\n\n  Install it manually:\n    %s",
					bin, err, strings.Join(install, " "))
			}
			if _, err := exec.LookPath(bin); err != nil {
				return fmt.Errorf("%s still is not on PATH after installing it", bin)
			}
			fmt.Fprintf(a.err, "\nanyctr: %s installed; continuing.\n\n", bin)
			return nil
		}
		fmt.Fprintln(a.err)
	}

	return fmt.Errorf("%s", composeMissingMessage(adapter.Name(), ext, args))
}

// composeMissingMessage explains an absent provider the same way whether anyctr
// is about to offer the install or has already been told no.
func composeMissingMessage(runtimeName string, ext runtime.ExternalComposer, args []string) string {
	subcommand := "up"
	if len(args) > 0 {
		subcommand = args[0]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "runtime %q ships no Compose implementation\n\n", runtimeName)
	fmt.Fprintf(&b, "  anyctr routes `compose` to %s, which is not installed:\n", ext.ComposeBinary())
	if install := ext.ComposeInstall(); len(install) > 0 {
		fmt.Fprintf(&b, "    %s\n", strings.Join(install, " "))
	}
	if src := ext.ComposeSource(); src != "" {
		fmt.Fprintf(&b, "    %s\n", src)
	}
	fmt.Fprintf(&b, "\n  Or select a runtime with Compose built in:\n")
	fmt.Fprintf(&b, "    anyctr --runtime=docker compose %s", subcommand)
	return b.String()
}

// confirm asks a yes/no question on stderr, so it cannot pollute piped stdout.
func (a *app) confirm(question string) bool {
	fmt.Fprintf(a.err, "%s [y/N] ", question)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

// interactive reports whether there is a terminal to ask questions on. Without
// one — in CI, in a pipeline — anyctr must fail with instructions instead of
// blocking forever on a prompt nobody will answer.
func interactive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func runInstaller(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stderr, os.Stderr
	return cmd.Run()
}

// hasComposePlugin reports whether `docker compose` exists. The probe is a real
// invocation because plugin discovery depends on Docker's own config, not on a
// path anyctr could guess.
func hasComposePlugin(dockerBin string) bool {
	cmd := exec.Command(dockerBin, "compose", "version")
	return cmd.Run() == nil
}
