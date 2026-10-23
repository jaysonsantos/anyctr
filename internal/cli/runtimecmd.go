package cli

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pedrospdc/anyctr/internal/detect"
	"github.com/pedrospdc/anyctr/internal/runtime"
)

// gatewayCommands are the commands whose flags are translated, and therefore the
// ones the capability matrix is built from.
var gatewayCommands = []string{
	"run", "ps", "images", "build", "pull", "push", "logs", "exec", "stop", "rm", "rmi", "tag",
}

func newRuntimeCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runtime",
		Short: "Inspect the container runtimes on this machine",
	}
	cmd.AddCommand(newRuntimeListCmd(a), newRuntimeCapsCmd(a))
	return cmd
}

func newRuntimeListCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List known runtimes and whether they are installed",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			found := detect.List(a.cfg)

			// Mark the runtime this machine would actually use.
			selected := ""
			if _, info, err := a.selectRuntime(); err == nil {
				selected = info.Name
			}

			w := tabwriter.NewWriter(a.out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "\tNAME\tSTATUS\tVERSION\tBINARY\tDESCRIPTION")
			for _, f := range found {
				marker := " "
				if f.Adapter.Name() == selected {
					marker = "*"
				}
				status, version, binary := "available", f.Version, f.Path
				if !f.Available {
					status, version, binary = f.Reason, "-", "-"
				}
				if version == "" {
					version = "unknown"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					marker, f.Adapter.Name(), status, version, binary, f.Adapter.Description())
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if selected != "" {
				fmt.Fprintf(a.out, "\n* selected for this machine (override with --runtime or ANYCTR_RUNTIME)\n")
			}
			return nil
		},
	}
}

func newRuntimeCapsCmd(a *app) *cobra.Command {
	var markdown bool
	var showAll bool
	cmd := &cobra.Command{
		Use:   "capabilities [COMMAND...]",
		Short: "Show how each runtime handles each Docker flag",
		Long: `Show how each runtime handles each Docker flag anyctr models.

By default only the flags that are not translated faithfully everywhere are
listed — those are the ones worth knowing about. Use --all for the full matrix,
and --markdown to regenerate docs/CAPABILITIES.md.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			commands := gatewayCommands
			if len(args) > 0 {
				commands = args
			}
			matrix := buildMatrix(cmd.Root(), commands)
			if markdown {
				return renderMatrixMarkdown(a, matrix)
			}
			return renderMatrixText(a, matrix, showAll)
		},
	}
	cmd.Flags().BoolVar(&markdown, "markdown", false, "Render as a Markdown document")
	cmd.Flags().BoolVar(&showAll, "all", false, "Include flags every runtime supports faithfully")
	return cmd
}

// cell is one runtime's handling of one flag.
type cell struct {
	Runtime string
	Support runtime.Support
}

// row is one flag across every runtime.
type row struct {
	Command string
	Flag    string
	Usage   string
	Cells   []cell
}

// buildMatrix derives the capability matrix from the cobra command tree, so the
// flags anyctr documents are exactly the flags anyctr accepts — there is no
// second list to keep in sync.
func buildMatrix(root *cobra.Command, commands []string) []row {
	adapters := detect.Adapters()
	caps := make(map[string]runtime.Caps, len(adapters))
	for _, ad := range adapters {
		caps[ad.Name()] = ad.Caps()
	}

	var rows []row
	for _, name := range commands {
		cmd := findCommand(root, name)
		if cmd == nil {
			continue
		}
		var flags []*pflag.Flag
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "help" {
				return
			}
			flags = append(flags, f)
		})
		sort.Slice(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })

		for _, f := range flags {
			r := row{Command: name, Flag: f.Name, Usage: f.Usage}
			for _, ad := range adapters {
				r.Cells = append(r.Cells, cell{Runtime: ad.Name(), Support: caps[ad.Name()].Lookup(name, f.Name)})
			}
			rows = append(rows, r)
		}
	}
	return rows
}

func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func (r row) faithfulEverywhere() bool {
	for _, c := range r.Cells {
		if c.Support.Level != runtime.Supported {
			return false
		}
	}
	return true
}

func symbol(l runtime.Level) string {
	switch l {
	case runtime.Supported:
		return "yes"
	case runtime.Limited:
		return "limited"
	case runtime.Divergent:
		return "differs"
	default:
		return "no"
	}
}

func renderMatrixText(a *app, rows []row, showAll bool) error {
	w := tabwriter.NewWriter(a.out, 0, 0, 2, ' ', 0)
	adapters := detect.Adapters()
	header := []string{"COMMAND", "FLAG"}
	for _, ad := range adapters {
		header = append(header, strings.ToUpper(ad.Name()))
	}
	fmt.Fprintln(w, strings.Join(header, "\t"))

	shown := 0
	var notes []string
	for _, r := range rows {
		if !showAll && r.faithfulEverywhere() {
			continue
		}
		shown++
		line := []string{r.Command, "--" + r.Flag}
		for _, c := range r.Cells {
			line = append(line, symbol(c.Support.Level))
			if c.Support.Note != "" {
				notes = append(notes, fmt.Sprintf("%s --%s on %s: %s", r.Command, r.Flag, c.Runtime, firstLine(c.Support.Note)))
			}
		}
		fmt.Fprintln(w, strings.Join(line, "\t"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if shown == 0 {
		fmt.Fprintln(a.out, "(every modelled flag translates faithfully on every runtime)")
		return nil
	}
	fmt.Fprintf(a.out, "\n%d flag(s) need attention. Details:\n\n", shown)
	for _, n := range notes {
		fmt.Fprintf(a.out, "  %s\n", n)
	}
	return nil
}

func renderMatrixMarkdown(a *app, rows []row) error {
	adapters := detect.Adapters()
	fmt.Fprintln(a.out, "# Capability matrix")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "Generated by `anyctr runtime capabilities --all --markdown`. Do not edit by hand.")
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "| symbol | meaning |")
	fmt.Fprintln(a.out, "| --- | --- |")
	fmt.Fprintln(a.out, "| yes | translates faithfully |")
	fmt.Fprintln(a.out, "| limited | translates, but over a narrower set of values |")
	fmt.Fprintln(a.out, "| differs | translated and emitted, but the semantics are not identical (anyctr warns) |")
	fmt.Fprintln(a.out, "| no | no faithful translation (anyctr fails, or warns under `--relax`) |")

	current := ""
	for _, r := range rows {
		if r.Command != current {
			current = r.Command
			fmt.Fprintf(a.out, "\n## anyctr %s\n\n", current)
			header := "| flag |"
			divider := "| --- |"
			for _, ad := range adapters {
				header += " " + ad.Name() + " |"
				divider += " --- |"
			}
			fmt.Fprintln(a.out, header+" notes |")
			fmt.Fprintln(a.out, divider+" --- |")
		}
		line := fmt.Sprintf("| `--%s` |", r.Flag)
		var notes []string
		for _, c := range r.Cells {
			line += " " + symbol(c.Support.Level) + " |"
			if c.Support.Note != "" {
				notes = append(notes, fmt.Sprintf("**%s**: %s", c.Runtime, firstLine(c.Support.Note)))
			}
		}
		fmt.Fprintln(a.out, line+" "+strings.Join(notes, "<br>")+" |")
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func newVersionCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the anyctr version and the runtime it would use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(a.out, "anyctr %s\n", cmd.Root().Version)
			_, info, err := a.selectRuntime()
			if err != nil {
				fmt.Fprintf(a.out, "runtime: none (%v)\n", firstLine(err.Error()))
				return nil
			}
			fmt.Fprintf(a.out, "runtime: %s (%s)\nbinary:  %s\n", info.Name, info.Label, info.Binary)
			return nil
		},
	}
}
