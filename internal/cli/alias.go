package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"

	"github.com/spf13/cobra"

	"github.com/pedrospdc/anyctr/internal/detect"
)

// aliasName is the command the rest of the world expects. anyctr already speaks
// Docker's convention, so answering to `docker` is the whole point: Makefiles,
// CI scripts and muscle memory keep working on a machine that has no Docker.
const aliasName = "docker"

// A symlink rather than a shell alias, deliberately: `make`, `npm run` and
// anything else that shells out use /bin/sh, which never sees the aliases in a
// .zshrc. A shell alias would work only when typed by hand, which is the case
// that needs the least help.
func newAliasCmd(a *app) *cobra.Command {
	var yes, force, remove bool
	var dir string

	cmd := &cobra.Command{
		Use:   "alias [OPTIONS]",
		Short: "Install a `docker` command that runs anyctr",
		Long: `Create a "docker" command that runs anyctr.

anyctr speaks Docker's convention, so pointing ` + "`docker`" + ` at it lets Makefiles,
scripts and habits keep working on a machine where Docker is not installed —
Apple's container or Podman answers instead.

This installs a symlink, not a shell alias: make and scripts run /bin/sh, which
does not see the aliases in your .zshrc. It never replaces a real Docker
installation unless you pass --force, and ` + "`anyctr alias --remove`" + ` undoes it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.alias(aliasOptions{yes: yes, force: force, remove: remove, dir: dir})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Don't ask for confirmation")
	cmd.Flags().BoolVar(&force, "force", false, "Replace an existing docker command")
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove the docker alias")
	cmd.Flags().StringVar(&dir, "dir", "", "Directory for the link (default: next to anyctr, or ~/.local/bin)")
	return flagErrors(cmd)
}

type aliasOptions struct {
	yes    bool
	force  bool
	remove bool
	dir    string
}

func (a *app) alias(opts aliasOptions) error {
	self, err := selfPath()
	if err != nil {
		return err
	}
	existing, _ := exec.LookPath(aliasName)
	ours := existing != "" && shimPointsToUs(existing, self)

	if opts.remove {
		if existing == "" || !ours {
			fmt.Fprintf(a.out, "No %s alias to anyctr found.\n", aliasName)
			return nil
		}
		if err := os.Remove(existing); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "Removed %s\n", existing)
		return nil
	}

	if ours {
		fmt.Fprintf(a.out, "%s already runs anyctr (%s).\n", aliasName, existing)
		return nil
	}
	// Shadowing a real Docker would silently reroute every container command on
	// the machine. That needs to be asked for explicitly.
	if existing != "" && !opts.force {
		fmt.Fprintf(a.out, "%s is already installed at %s; leaving it alone (use --force to override).\n",
			aliasName, existing)
		return nil
	}

	if !opts.yes {
		question := fmt.Sprintf("No %s command found. Create one that runs anyctr (for Makefiles and scripts)?", aliasName)
		ok, err := confirm(a.out, question, true)
		if err != nil {
			fmt.Fprintf(a.out, "No terminal to ask on; run `anyctr alias --yes` to create the %s command.\n", aliasName)
			return nil
		}
		if !ok {
			fmt.Fprintln(a.out, "Skipped. You can run `anyctr alias` at any time.")
			return nil
		}
	}

	target := opts.dir
	if target == "" {
		target = filepath.Dir(self)
		if !writable(target) {
			home, _ := os.UserHomeDir()
			target = filepath.Join(home, ".local", "bin")
		}
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}

	link := filepath.Join(target, aliasFileName())
	if opts.force {
		_ = os.Remove(link)
	}
	if err := createShim(self, link); err != nil {
		return fmt.Errorf("creating %s: %w", link, err)
	}

	fmt.Fprintf(a.out, "Created %s -> %s\n", link, self)
	if !onPath(target) {
		fmt.Fprintf(a.out, "Note: %s is not on your PATH; add it to use %s.\n", target, aliasName)
	}
	fmt.Fprintf(a.out, "\n`%s` now runs anyctr. Undo it with `anyctr alias --remove`.\n", aliasName)
	return nil
}

// aliasFileName is the file to create. Windows gets a .cmd shim because symlinks
// there need developer mode or an elevated prompt.
func aliasFileName() string {
	if goruntime.GOOS == "windows" {
		return aliasName + ".cmd"
	}
	return aliasName
}

func createShim(self, link string) error {
	if goruntime.GOOS == "windows" {
		return os.WriteFile(link, []byte("@echo off\r\n\""+self+"\" %*\r\n"), 0o755)
	}
	return os.Symlink(self, link)
}

// shimPointsToUs reports whether an existing command is anyctr wearing the
// alias, rather than a real Docker that must be left alone.
func shimPointsToUs(path, self string) bool {
	return detect.IsSelf(path) || sameFile(path, self)
}

// selfPath prefers the PATH entry (Homebrew's bin symlink, say) over the
// resolved file in a versioned directory, so upgrades keep the alias working.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := exec.LookPath("anyctr"); err == nil && sameFile(p, exe) {
		return filepath.Abs(p)
	}
	return exe, nil
}

func sameFile(a, b string) bool { return detect.SameFile(a, b) }

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".anyctr-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

func onPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(p) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

// suggestAlias offers the alias where an engineer will actually see it: after
// `anyctr runtime list`, on a machine that has a runtime but no docker command.
func (a *app) suggestAlias(found []detect.Found) {
	if _, err := exec.LookPath(aliasName); err == nil {
		return // a docker command already exists, ours or theirs
	}
	for _, f := range found {
		if !f.Available {
			continue
		}
		fmt.Fprintf(a.out, "\nNo `%s` command on this machine. To let Makefiles and scripts\n"+
			"use %s through anyctr:\n    anyctr alias\n", aliasName, f.Adapter.Name())
		return
	}
}
