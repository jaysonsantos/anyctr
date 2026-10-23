// Package runtime defines the adapter contract every container CLI plugs into,
// plus the capability model that makes flag translation honest.
//
// An adapter never executes anything. It turns a spec.Op into an Invocation —
// a fully formed argv — which callers can either run or print (`--explain`).
// That split is what makes the whole translation layer testable without any
// runtime installed.
package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pedrospdc/anyctr/internal/diag"
	"github.com/pedrospdc/anyctr/internal/spec"
)

// Invocation is a translated command line, ready to execute.
type Invocation struct {
	Bin   string   // binary to execute (resolved path when known)
	Args  []string // arguments, not including Bin
	TTY   bool     // the child expects to own the terminal
	Stdin bool     // the child expects stdin to stay open
}

// String renders the invocation the way a human would type it.
func (i Invocation) String() string {
	parts := make([]string, 0, len(i.Args)+1)
	parts = append(parts, shellQuote(i.Bin))
	for _, a := range i.Args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'$`\\*?|&;<>()[]{}#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Info identifies the runtime a translation is targeting.
type Info struct {
	Name    string // adapter name, e.g. "container"
	Binary  string // resolved binary path, or bare name when unresolved
	Version string // as reported by the CLI, e.g. "1.2.2"
	Label   string // human label, e.g. "apple/container 1.2.2"
}

// Level says how a runtime handles a given Docker flag.
type Level int

const (
	// Supported means the flag translates faithfully.
	Supported Level = iota
	// Limited means the flag translates, but only over a narrower set of values
	// than Docker accepts (e.g. `--format` as an enum instead of a Go template).
	// The adapter validates the value and rejects precisely what it cannot honour,
	// so there is nothing to warn about when the value is fine.
	Limited
	// Divergent means the flag translates, but the semantics are not identical.
	// anyctr emits it and warns, because silence here is how engineers get
	// surprised in production.
	Divergent
	// Unsupported means there is no faithful translation.
	Unsupported
)

func (l Level) String() string {
	switch l {
	case Supported:
		return "supported"
	case Limited:
		return "limited"
	case Divergent:
		return "divergent"
	default:
		return "unsupported"
	}
}

// Support describes one runtime's handling of one flag.
type Support struct {
	Level        Level
	Note         string   // for Divergent: how it differs. for Unsupported: why.
	Alternatives []string // concrete next steps for the engineer
}

// Reason returns the explanation carried by an Unsupported entry.
func (s Support) Reason() string { return s.Note }

// Yes marks a faithful translation. It is the default for unlisted flags, so
// adapters only have to enumerate their exceptions.
func Yes() Support { return Support{Level: Supported} }

// Ltd marks a flag whose accepted values are narrower than Docker's. The adapter
// is responsible for rejecting values it cannot translate.
func Ltd(note string) Support { return Support{Level: Limited, Note: note} }

// Div marks a flag that is emitted but behaves differently.
func Div(note string) Support { return Support{Level: Divergent, Note: note} }

// No marks a flag with no faithful translation.
func No(reason string, alternatives ...string) Support {
	return Support{Level: Unsupported, Note: reason, Alternatives: alternatives}
}

// Caps maps "<command>.<flag>" to the runtime's handling of it. Absent keys are
// Supported: adapters only emit flags they know about, so the map's job is to
// record the gaps and the surprises — which is also exactly what the capability
// matrix in the docs is generated from.
type Caps map[string]Support

// Lookup resolves a capability, falling back to the wildcard "*.<flag>" entry
// (used for flags that are missing across every command of a runtime).
func (c Caps) Lookup(command, flag string) Support {
	if s, ok := c[command+"."+flag]; ok {
		return s
	}
	if s, ok := c["*."+flag]; ok {
		return s
	}
	return Yes()
}

// Keys returns the capability keys in stable order (for docs and tests).
func (c Caps) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Policy controls what happens when a flag cannot be translated faithfully.
type Policy struct {
	// Relax drops unsupported flags with a warning instead of failing.
	Relax bool
	// Flags overrides behaviour per canonical flag name: "error" or "drop".
	// Keys may be "run.restart" or just "restart".
	Flags map[string]string
}

func (p Policy) drop(command, flag string) bool {
	if p.Flags != nil {
		if v, ok := p.Flags[command+"."+flag]; ok {
			return v == "drop"
		}
		if v, ok := p.Flags[flag]; ok {
			return v == "drop"
		}
	}
	return p.Relax
}

// Tr carries the state of a single translation: which runtime is being targeted,
// what its capabilities are, and what went wrong or needs flagging along the way.
type Tr struct {
	Info Info

	caps    Caps
	policy  Policy
	command string
	warns   []string
	err     error
}

// NewTr starts a translation for one operation.
func NewTr(info Info, caps Caps, policy Policy, command string) *Tr {
	return &Tr{Info: info, caps: caps, policy: policy, command: command}
}

// Allow reports whether the adapter may emit the given canonical flag. It
// records a warning for divergent flags and an error (or a warning, under
// --relax) for unsupported ones, so adapters read as a flat list of
// `if set && allowed { emit }` lines with no error plumbing.
func (t *Tr) Allow(flag string) bool {
	s := t.caps.Lookup(t.command, flag)
	switch s.Level {
	case Supported, Limited:
		return true
	case Divergent:
		t.Warnf("--%s: %s", flag, s.Note)
		return true
	default:
		if t.policy.drop(t.command, flag) {
			t.Warnf("--%s is not supported by %q and was dropped: %s", flag, t.Info.Name, s.Reason())
			return false
		}
		t.Fail(&diag.Unsupported{
			Flag:         flag,
			Command:      t.command,
			Runtime:      t.Info.Name,
			RuntimeLabel: t.Info.Label,
			Reason:       s.Reason(),
			Alternatives: s.Alternatives,
		})
		return false
	}
}

// Reject fails the translation for a flag whose *value* (not presence) cannot be
// honoured, e.g. a fractional --cpus on a runtime that only takes whole CPUs.
func (t *Tr) Reject(flag, reason string, alternatives ...string) {
	if t.policy.drop(t.command, flag) {
		t.Warnf("--%s was dropped: %s", flag, reason)
		return
	}
	t.Fail(&diag.Unsupported{
		Flag:         flag,
		Command:      t.command,
		Runtime:      t.Info.Name,
		RuntimeLabel: t.Info.Label,
		Reason:       reason,
		Alternatives: alternatives,
	})
}

// RejectArg fails the translation for a positional argument the runtime cannot
// accept, e.g. `docker images nginx`, where the runtime's list command takes no
// reference filter.
func (t *Tr) RejectArg(name, reason string, alternatives ...string) {
	if t.policy.drop(t.command, name) {
		t.Warnf("%s was dropped: %s", name, reason)
		return
	}
	t.Fail(&diag.Unsupported{
		Flag:         name,
		Arg:          true,
		Command:      t.command,
		Runtime:      t.Info.Name,
		RuntimeLabel: t.Info.Label,
		Reason:       reason,
		Alternatives: alternatives,
	})
}

// Warnf records a warning to be shown on stderr before the command runs.
func (t *Tr) Warnf(format string, args ...any) {
	t.warns = append(t.warns, fmt.Sprintf(format, args...))
}

// Fail records the first translation error; later ones are dropped so the
// engineer sees the first thing that actually went wrong.
func (t *Tr) Fail(err error) {
	if t.err == nil {
		t.err = err
	}
}

// Err returns the translation error, if any.
func (t *Tr) Err() error { return t.err }

// Warnings returns the accumulated warnings.
func (t *Tr) Warnings() []string { return t.warns }

// Adapter translates operations for one container CLI.
type Adapter interface {
	// Name is the runtime's anyctr identifier, e.g. "container".
	Name() string
	// Binary is the executable anyctr looks for on PATH.
	Binary() string
	// Aliases lists fallback binary names, tried in order after Binary.
	Aliases() []string
	// Description is a one-line human summary for `anyctr runtime list`.
	Description() string
	// Platforms lists the GOOS values this runtime can exist on; nil means any.
	Platforms() []string
	// Caps reports the runtime's flag capabilities.
	Caps() Caps
	// Translate turns an operation into an invocation. Unsupported flags are
	// reported through tr, not returned, so a single translation can collect
	// every warning before the first error stops it.
	Translate(op spec.Op, tr *Tr) (Invocation, error)
	// ParseVersion extracts a version from the CLI's `--version` output.
	ParseVersion(raw string) string
	// Label renders the human runtime label for a version, e.g.
	// "apple/container 1.2.2".
	Label(version string) string
}

// ErrUnsupportedOp reports an operation the runtime has no equivalent for at all
// (as opposed to a single unsupported flag).
type ErrUnsupportedOp struct {
	Op           string
	Runtime      string
	Reason       string
	Alternatives []string
}

func (e *ErrUnsupportedOp) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%q is not supported by runtime %q", e.Op, e.Runtime)
	if e.Reason != "" {
		fmt.Fprintf(&b, "\n\n  %s", e.Reason)
	}
	for _, alt := range e.Alternatives {
		fmt.Fprintf(&b, "\n  %s", alt)
	}
	return b.String()
}

// Terminal reports how an operation expects to use the terminal, so the executor
// knows whether to keep stdin open and whether the child owns the tty. This is a
// property of the operation, not of the runtime, so every adapter shares it.
func Terminal(op spec.Op) (tty, stdin bool) {
	switch s := op.(type) {
	case *spec.Run:
		return s.TTY, s.Interactive
	case *spec.Exec:
		return s.TTY, s.Interactive
	case *spec.Build:
		return true, false
	case *spec.Logs:
		return s.Follow, false
	case *spec.Compose:
		return true, true
	}
	return false, false
}

// Command reports the anyctr command being translated, for messages that need to
// suggest a corrected invocation.
func (t *Tr) Command() string { return t.command }
