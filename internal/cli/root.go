// Package cli defines anyctr's command surface.
//
// The surface is Docker's, deliberately: it is the convention engineers already
// know, and the one their scripts and muscle memory are written against. Each
// command parses into a spec.Op and hands it to the gateway, which selects a
// runtime, translates, and either runs or explains the result. No command talks
// to a runtime directly.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pedrospdc/anyctr/internal/config"
	"github.com/pedrospdc/anyctr/internal/detect"
	"github.com/pedrospdc/anyctr/internal/diag"
	"github.com/pedrospdc/anyctr/internal/procexec"
	"github.com/pedrospdc/anyctr/internal/runtime"
	"github.com/pedrospdc/anyctr/internal/spec"
)

// cliErrorCode is the exit status for anyctr's own errors — a bad flag, an
// unsupported translation, no runtime found. It matches Docker's convention, so
// scripts can tell "the gateway refused" from "the container failed".
const cliErrorCode = 125

// app carries the state every command needs: the resolved config and the global
// switches that change how translation behaves.
type app struct {
	configPath  string
	runtimeName string
	explain     bool
	relax       bool

	cfg *config.Config
	out io.Writer
	err io.Writer
}

// NewRootCmd builds the command tree. Version is injected at build time.
func NewRootCmd(version string) *cobra.Command {
	return newRootCmd(version, &app{out: os.Stdout, err: os.Stderr})
}

// newRootCmd builds the tree around a given app, so tests can capture output.
func newRootCmd(version string, a *app) *cobra.Command {
	root := &cobra.Command{
		Use:   "anyctr",
		Short: "One CLI for any container runtime",
		Long: `anyctr is a gateway for container runtime CLIs.

You type Docker's convention; anyctr translates it for whichever runtime is
installed — docker, podman, or Apple's container — and refuses, loudly, when a
flag has no faithful equivalent instead of silently changing what you asked for.`,
		SilenceUsage:      true,
		SilenceErrors:     true,
		Version:           version,
		PersistentPreRunE: a.load,
	}

	pf := root.PersistentFlags()
	pf.StringVar(&a.configPath, "config", "", "path to the anyctr config file")
	pf.StringVar(&a.runtimeName, "runtime", "", "runtime to target (docker, podman, container); default: auto-detect")
	pf.BoolVar(&a.explain, "explain", false, "print the translated runtime command instead of running it")
	pf.BoolVar(&a.relax, "relax", false, "drop flags the runtime cannot honour, with a warning, instead of failing")

	root.AddCommand(
		newRunCmd(a),
		newPsCmd(a),
		newImagesCmd(a),
		newBuildCmd(a),
		newPullCmd(a),
		newPushCmd(a),
		newLogsCmd(a),
		newExecCmd(a),
		newStopCmd(a),
		newRmCmd(a),
		newRmiCmd(a),
		newTagCmd(a),
		newComposeCmd(a),
		newRawCmd(a),
		newRuntimeCmd(a),
		newConfigCmd(a),
		newVersionCmd(a),
	)
	return root
}

// Execute runs anyctr and returns the process exit status.
func Execute(version string) int {
	root := NewRootCmd(version)
	err := root.Execute()
	if err == nil {
		return 0
	}

	// A non-zero status from the runtime is not anyctr's error to report: the
	// runtime already said what went wrong on stderr.
	var exit *procexec.ExitCodeError
	if errors.As(err, &exit) {
		return exit.Code
	}
	fmt.Fprintf(os.Stderr, "anyctr: error: %v\n", err)
	return cliErrorCode
}

// load resolves configuration and environment defaults before any command runs.
func (a *app) load(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	a.cfg = cfg

	if a.runtimeName == "" {
		a.runtimeName = os.Getenv("ANYCTR_RUNTIME")
	}
	if !a.explain && os.Getenv("ANYCTR_EXPLAIN") != "" {
		a.explain = true
	}
	if !a.relax && os.Getenv("ANYCTR_RELAX") != "" {
		a.relax = true
	}
	return nil
}

// selectRuntime resolves the runtime this invocation targets.
func (a *app) selectRuntime() (runtime.Adapter, runtime.Info, error) {
	return detect.Select(a.cfg, detect.Request{Name: a.runtimeName, Explain: a.explain})
}

// policy combines the config's strictness with this invocation's overrides.
func (a *app) policy() runtime.Policy {
	strict := true
	var flags map[string]string
	if a.cfg != nil {
		strict = a.cfg.Strict
		flags = a.cfg.Flags
	}
	return runtime.Policy{Relax: a.relax || !strict, Flags: flags}
}

// gateway is the single path every container command takes: select, translate,
// then explain or execute.
func (a *app) gateway(op spec.Op) error {
	adapter, info, err := a.selectRuntime()
	if err != nil {
		return err
	}

	tr := runtime.NewTr(info, adapter.Caps(), a.policy(), op.Command())
	inv, err := adapter.Translate(op, tr)
	a.printWarnings(tr.Warnings())
	if err != nil {
		return err
	}
	return a.dispatch(info, inv)
}

// dispatch either prints the translation or runs it.
func (a *app) dispatch(info runtime.Info, inv runtime.Invocation) error {
	if a.explain {
		fmt.Fprintf(a.out, "runtime: %s (%s)\n", info.Name, info.Label)
		fmt.Fprintf(a.out, "binary:  %s\n", inv.Bin)
		fmt.Fprintf(a.out, "command: %s\n", inv.String())
		return nil
	}
	code, err := procexec.Run(inv)
	if err != nil {
		return err
	}
	if code != 0 {
		return &procexec.ExitCodeError{Code: code}
	}
	return nil
}

func (a *app) printWarnings(warnings []string) {
	for _, w := range warnings {
		fmt.Fprintf(a.err, "anyctr: warning: %s\n", w)
	}
}

// consumeGlobals pulls anyctr's own global flags off the front of a passthrough
// command's arguments.
//
// `compose` and `raw` forward everything verbatim, which means cobra cannot parse
// flags for them at all — otherwise `anyctr compose -f compose.yaml up` would be
// read as an anyctr flag. The cost is that root's persistent flags arrive here as
// plain arguments, so they are recognised explicitly, and only while they lead the
// list. Everything from the first non-global token onwards belongs to the runtime.
func (a *app) consumeGlobals(args []string) []string {
	for len(args) > 0 {
		arg := args[0]
		name, value, hasValue := strings.Cut(arg, "=")

		takeValue := func() (string, bool) {
			if hasValue {
				args = args[1:]
				return value, true
			}
			if len(args) < 2 {
				return "", false
			}
			v := args[1]
			args = args[2:]
			return v, true
		}

		switch name {
		case "--explain":
			a.explain = true
			args = args[1:]
		case "--relax":
			a.relax = true
			args = args[1:]
		case "--runtime":
			v, ok := takeValue()
			if !ok {
				return args
			}
			a.runtimeName = v
		case "--config":
			v, ok := takeValue()
			if !ok {
				return args
			}
			// The config was already loaded from the default path, so reload it.
			a.configPath = v
			if cfg, err := config.Load(v); err == nil {
				a.cfg = cfg
			}
		default:
			return args
		}
	}
	return args
}

// dockerStyle configures a command the way the Docker CLI configures its own:
// flags stop at the first positional argument, so everything after the image or
// container name belongs to the containerized process, not to anyctr.
func dockerStyle(cmd *cobra.Command) {
	cmd.Flags().SetInterspersed(false)
}

// reserveHelpShorthand frees up -h for a command that needs it for --hostname,
// exactly as the Docker CLI does for `docker run`.
func reserveHelpShorthand(cmd *cobra.Command) {
	cmd.Flags().Bool("help", false, "help for "+cmd.Name())
}

// unsupportedFlagHint tells engineers where the escape hatch is when they reach
// for a flag anyctr does not model yet.
func unsupportedFlagHint(cmd *cobra.Command, err error) error {
	return fmt.Errorf("%w\n\n"+
		"  anyctr models Docker's flags explicitly so it can translate them.\n"+
		"  For a flag it does not map yet, pass the command straight through:\n"+
		"    anyctr raw %s ...", err, cmd.Name())
}

// flagErrors installs the hint above on a command and its children.
func flagErrors(cmd *cobra.Command) *cobra.Command {
	cmd.SetFlagErrorFunc(unsupportedFlagHint)
	return cmd
}

// requireArgs is a small wrapper that produces Docker-flavoured argument errors.
func requireArgs(cmd string, min int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < min {
			return fmt.Errorf(`"anyctr %s" requires at least %d argument%s

Usage:  anyctr %s`, cmd, min, plural(min), usage)
		}
		return nil
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// changed reports whether a flag was explicitly set, which is how anyctr tells
// `--entrypoint=""` (clear it) from an absent --entrypoint (leave it alone).
func changed(cmd *cobra.Command, name string) bool {
	return cmd.Flags().Changed(name)
}

// isUnsupported reports whether an error is a translation refusal, used by tests.
func isUnsupported(err error) bool {
	var u *diag.Unsupported
	return errors.As(err, &u)
}

// runtimeNames renders the known runtime list for help text.
func runtimeNames() string {
	return strings.Join(detect.Names(), ", ")
}
