//go:build conformance

// Conformance tests check anyctr's capability matrix against the help surface of
// the runtimes actually installed on this machine:
//
//	go test -tags conformance ./internal/cli/            # every runtime found
//	ANYCTR_RUNTIME=podman go test -tags conformance ./...  # just one
//
// They answer the question --explain tests cannot: not "is our argv what we
// intended" but "does the runtime still accept it". The checks run in both
// directions, and the second one is the valuable half — when Apple ships
// --restart in some future release, a matrix that still refuses it is lying to
// engineers, and nothing else in the suite would notice.
//
// Only `--help` is invoked, so no container is ever started and no engine needs
// to be running.
package cli

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pedrospdc/anyctr/internal/config"
	"github.com/pedrospdc/anyctr/internal/detect"
	"github.com/pedrospdc/anyctr/internal/runtime"
	"github.com/spf13/pflag"
)

// positionals are the arguments each command needs to parse, so a flag can be
// exercised through the real CLI path.
var positionals = map[string][]string{
	"run":    {"IMAGE"},
	"ps":     {},
	"images": {},
	"build":  {"CTX"},
	"pull":   {"REF"},
	"push":   {"REF"},
	"logs":   {"CTR"},
	"exec":   {"CTR", "CMD"},
	"stop":   {"CTR"},
	"rm":     {"CTR"},
	"rmi":    {"IMG"},
	"tag":    {"SRC", "DST"},
}

// flagValues keeps generated values inside each flag's accepted domain, so a
// "limited" flag is exercised rather than rejected on its value.
var flagValues = map[string]string{
	"cpus":     "2",
	"format":   "json",
	"last":     "3",
	"memory":   "512m",
	"network":  "bridge",
	"output":   "type=oci",
	"platform": "linux/amd64",
	"progress": "plain",
	"pull":     "always",
	"restart":  "always",
	"secret":   "id=s,src=/dev/null",
	"ssh":      "default",
	"tail":     "20",
	"time":     "5",
}

// helpOmissions lists flags a runtime genuinely accepts but does not print in
// `--help` (hidden or deprecated flags). Each entry needs a reason: this map is
// how a real conformance failure gets silenced, so it must stay small and argued.
var helpOmissions = map[string]string{}

func TestCapabilityMatrixMatchesInstalledRuntimes(t *testing.T) {
	cfg := config.Default()
	found := detect.List(cfg)

	tested := 0
	for _, f := range found {
		if !f.Available {
			t.Logf("skipping %s: %s", f.Adapter.Name(), f.Reason)
			continue
		}
		tested++
		t.Run(f.Adapter.Name(), func(t *testing.T) {
			checkRuntime(t, f)
		})
	}
	if tested == 0 {
		t.Skip("no container runtime installed; nothing to check conformance against")
	}
}

func checkRuntime(t *testing.T, f detect.Found) {
	caps := f.Adapter.Caps()
	root := newRootCmd("test", &app{out: &bytes.Buffer{}, err: &bytes.Buffer{}})

	for _, command := range gatewayCommands {
		cmd := findCommand(root, command)
		if cmd == nil {
			t.Fatalf("command %q is in gatewayCommands but not in the command tree", command)
		}

		baseline, ok := translate(t, f, command, nil)
		if !ok {
			continue
		}
		sub := subcommand(baseline, positionals[command])
		help, err := helpText(f.Path, sub)
		if err != nil {
			t.Errorf("%s %s --help failed: %v", filepath.Base(f.Path), strings.Join(sub, " "), err)
			continue
		}

		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if flag.Name == "help" {
				return
			}
			t.Run(command+"/--"+flag.Name, func(t *testing.T) {
				checkFlag(t, f, caps, help, command, flag, baseline)
			})
		})
	}
}

func checkFlag(t *testing.T, f detect.Found, caps runtime.Caps, help, command string, flag *pflag.Flag, baseline []string) {
	support := caps.Lookup(command, flag.Name)

	args := []string{"--" + flag.Name}
	if flag.Value.Type() != "bool" {
		args[0] += "=" + flagValue(flag.Name)
	}
	emitted, ok := translate(t, f, command, args)
	if !ok {
		return
	}
	added := addedFlags(baseline, emitted)

	if support.Level == runtime.Unsupported {
		if len(added) > 0 {
			t.Errorf("matrix says --%s is unsupported, but the adapter emitted %v", flag.Name, added)
		}
		// The drift that matters: the runtime grew the flag and the refusal is
		// now wrong.
		if mentions(help, "--"+flag.Name) {
			t.Errorf("matrix refuses --%s, but `%s %s --help` lists it — the runtime gained support and the refusal is now a lie",
				flag.Name, filepath.Base(f.Path), command)
		}
		return
	}

	if len(added) == 0 {
		t.Errorf("matrix says --%s is %s, but the adapter emitted nothing for it",
			flag.Name, support.Level)
		return
	}
	for _, token := range added {
		if mentions(help, token) {
			continue
		}
		if reason, excused := helpOmissions[f.Adapter.Name()+" "+command+" "+token]; excused {
			t.Logf("known omission (%s): %s", token, reason)
			continue
		}
		t.Errorf("anyctr emits %s for --%s, but `%s %s --help` does not list it",
			token, flag.Name, filepath.Base(f.Path), strings.Join(subcommand(baseline, positionals[command]), " "))
	}
}

// translate runs the real CLI in --explain mode and returns the argv it would
// have executed. --relax keeps unsupported flags from aborting the sweep.
func translate(t *testing.T, f detect.Found, command string, extra []string) ([]string, bool) {
	t.Helper()
	var out, errBuf bytes.Buffer
	a := &app{out: &out, err: &errBuf}
	root := newRootCmd("test", a)

	argv := []string{"--explain", "--relax", "--runtime=" + f.Adapter.Name(), command}
	argv = append(argv, extra...)
	argv = append(argv, positionals[command]...)
	root.SetArgs(argv)
	root.SetOut(&out)
	root.SetErr(&errBuf)

	if err := root.Execute(); err != nil {
		t.Errorf("translating %v failed: %v", argv, err)
		return nil, false
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "command: ") {
			fields := strings.Fields(strings.TrimPrefix(line, "command: "))
			return fields[1:], true // drop the binary
		}
	}
	t.Errorf("no translated command in output:\n%s", out.String())
	return nil, false
}

// subcommand is the leading run of non-flag, non-positional tokens: `image list`
// for `anyctr images` on Apple's CLI. Deriving it from the translation means the
// help we check is the help for the command anyctr actually invokes.
func subcommand(args, placeholders []string) []string {
	var sub []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") || contains(placeholders, a) {
			break
		}
		sub = append(sub, a)
	}
	return sub
}

// addedFlags reports the flag tokens a translation gained over its baseline.
func addedFlags(baseline, emitted []string) []string {
	seen := map[string]int{}
	for _, a := range baseline {
		if strings.HasPrefix(a, "-") {
			seen[flagToken(a)]++
		}
	}
	var added []string
	for _, a := range emitted {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		token := flagToken(a)
		if seen[token] > 0 {
			seen[token]--
			continue
		}
		added = append(added, token)
	}
	return added
}

// flagToken strips a joined value: `--env=FOO=bar` is the `--env` flag.
func flagToken(arg string) string {
	if i := strings.IndexByte(arg, '='); i >= 0 {
		return arg[:i]
	}
	return arg
}

// mentions reports whether help text lists a flag, without matching --all inside
// --all-tags.
func mentions(help, token string) bool {
	re := regexp.MustCompile(`(^|[^-\w])` + regexp.QuoteMeta(token) + `($|[^-\w])`)
	return re.MatchString(help)
}

func helpText(bin string, sub []string) (string, error) {
	out, err := exec.Command(bin, append(append([]string{}, sub...), "--help")...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return "", fmt.Errorf("%w: %s", err, out)
	}
	return string(out), nil
}

func flagValue(name string) string {
	if v, ok := flagValues[name]; ok {
		return v
	}
	return "VALUE"
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
