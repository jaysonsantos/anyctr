// Package diag holds anyctr's user-facing error and warning types.
//
// The gateway's most important job is to never let a command mean something
// different than the engineer asked for. When a Docker flag has no faithful
// equivalent on the selected runtime, translation fails with an Unsupported
// error that names the flag, the runtime, why it cannot be honoured, and the
// ways forward.
package diag

import (
	"fmt"
	"strings"
)

// Unsupported reports a Docker flag that the selected runtime cannot honour.
type Unsupported struct {
	Flag         string   // canonical long flag name, without dashes
	Arg          bool     // Flag names a positional argument, not a flag
	Command      string   // anyctr command it was used with, e.g. "run"
	Runtime      string   // adapter name, e.g. "container"
	RuntimeLabel string   // human label, e.g. "apple/container 1.2.2"
	Reason       string   // why the runtime cannot do this
	Alternatives []string // concrete next steps, most useful first
}

func (e *Unsupported) Error() string {
	var b strings.Builder
	label := e.RuntimeLabel
	if label == "" {
		label = e.Runtime
	}
	if e.Arg {
		fmt.Fprintf(&b, "%s is not supported by runtime %q (%s)", e.Flag, e.Runtime, label)
	} else {
		fmt.Fprintf(&b, "--%s is not supported by runtime %q (%s)", e.Flag, e.Runtime, label)
	}
	if e.Reason != "" {
		fmt.Fprintf(&b, "\n\n  %s", e.Reason)
	}
	for _, alt := range e.Alternatives {
		fmt.Fprintf(&b, "\n  %s", alt)
	}
	fmt.Fprintf(&b, "\n\n  To drop unsupported flags instead of failing:")
	fmt.Fprintf(&b, "\n    anyctr --relax %s ...        (warns on stderr)", e.Command)
	fmt.Fprintf(&b, "\n    anyctr config set strict false")
	return b.String()
}

// NoRuntime reports that no supported runtime CLI could be found.
type NoRuntime struct {
	Searched []string
}

func (e *NoRuntime) Error() string {
	return fmt.Sprintf("no container runtime found on PATH (looked for: %s)\n\n"+
		"  Install one of them, or pin an explicit binary in your config:\n"+
		"    anyctr config set runtime docker\n\n"+
		"  See what anyctr can find with:\n"+
		"    anyctr runtime list", strings.Join(e.Searched, ", "))
}

// UnknownRuntime reports an explicit --runtime selection anyctr does not know.
type UnknownRuntime struct {
	Name  string
	Known []string
}

func (e *UnknownRuntime) Error() string {
	return fmt.Sprintf("unknown runtime %q (known: %s)", e.Name, strings.Join(e.Known, ", "))
}

// RuntimeUnavailable reports a known runtime whose CLI is not installed.
type RuntimeUnavailable struct {
	Name   string
	Binary string
	Hint   string
}

func (e *RuntimeUnavailable) Error() string {
	msg := fmt.Sprintf("runtime %q selected, but %q was not found on PATH", e.Name, e.Binary)
	if e.Hint != "" {
		msg += "\n\n  " + e.Hint
	}
	msg += "\n\n  Use `anyctr runtime list` to see what is available here," +
		"\n  or `anyctr --explain` to translate a command without running it."
	return msg
}
