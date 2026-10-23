package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newConfigCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and write anyctr's configuration",
		Long: `Read and write anyctr's configuration.

anyctr needs no configuration to work. The file exists to pin a runtime for a
repository or a machine, to relax strictness for a specific flag, or to point at
a binary that is not on PATH:

  anyctr config set runtime podman
  anyctr config set strict false
  anyctr config set flags.restart drop
  anyctr config set binaries.docker /opt/homebrew/bin/docker`,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "path",
			Short: "Print the path of the config file",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				fmt.Fprintln(a.out, a.cfg.FilePath())
				return nil
			},
		},
		&cobra.Command{
			Use:   "show",
			Short: "Print the effective configuration",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				rendered, err := a.cfg.Render()
				if err != nil {
					return err
				}
				fmt.Fprint(a.out, rendered)
				return nil
			},
		},
		&cobra.Command{
			Use:   "get KEY",
			Short: "Print one configuration value",
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				v, err := a.cfg.Get(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintln(a.out, v)
				return nil
			},
		},
		&cobra.Command{
			Use:   "set KEY VALUE",
			Short: "Set one configuration value",
			Args:  cobra.ExactArgs(2),
			RunE: func(_ *cobra.Command, args []string) error {
				if err := a.cfg.Set(args[0], args[1]); err != nil {
					return err
				}
				if err := a.cfg.Save(); err != nil {
					return err
				}
				fmt.Fprintf(a.out, "%s = %s (%s)\n", args[0], args[1], a.cfg.FilePath())
				return nil
			},
		},
		&cobra.Command{
			Use:   "unset KEY",
			Short: "Remove one configuration value",
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				if err := a.cfg.Set(args[0], ""); err != nil {
					return err
				}
				if err := a.cfg.Save(); err != nil {
					return err
				}
				fmt.Fprintf(a.out, "unset %s (%s)\n", args[0], a.cfg.FilePath())
				return nil
			},
		},
	)
	return cmd
}
