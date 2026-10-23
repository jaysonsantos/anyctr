package cli

import (
	"github.com/spf13/cobra"

	"github.com/pedrospdc/anyctr/internal/runtime"
)

// newRawCmd is the escape hatch. anyctr models Docker's flags explicitly so it
// can translate them honestly, which means it will always lag behind the full
// surface of four runtimes. `raw` forwards arguments to the selected runtime with
// no translation at all, so no engineer is ever blocked by a missing mapping.
func newRawCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "raw [ARG...]",
		Short: "Run a command on the selected runtime with no translation",
		Long: `Forward arguments to the selected runtime verbatim.

Use this for runtime-specific features anyctr does not model — Apple's
--rosetta, podman's --pod, a flag added last week:

  anyctr raw run --rosetta --rm -it alpine sh
  anyctr --runtime=podman raw pod create --name dev

Nothing is translated, so nothing is portable: a raw command means whatever the
selected runtime means by it.`,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			args = a.consumeGlobals(args)
			if len(args) == 0 {
				return cmd.Help()
			}
			return a.raw(args)
		},
	}
	return cmd
}

func (a *app) raw(args []string) error {
	_, info, err := a.selectRuntime()
	if err != nil {
		return err
	}
	return a.dispatch(info, runtime.Invocation{
		Bin:   info.Binary,
		Args:  args,
		TTY:   true,
		Stdin: true,
	})
}
