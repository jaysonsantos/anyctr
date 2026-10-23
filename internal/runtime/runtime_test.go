package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/pedrospdc/anyctr/internal/diag"
)

func testInfo() Info {
	return Info{Name: "container", Binary: "container", Version: "1.2.2", Label: "apple/container 1.2.2"}
}

func TestAllowSupported(t *testing.T) {
	tr := NewTr(testInfo(), Caps{}, Policy{}, "run")
	if !tr.Allow("publish") {
		t.Fatal("unlisted flags must default to supported")
	}
	if tr.Err() != nil || len(tr.Warnings()) != 0 {
		t.Fatal("a supported flag must not produce diagnostics")
	}
}

func TestAllowUnsupportedFailsStrictly(t *testing.T) {
	caps := Caps{"run.restart": No("no restart policy", "Nearest option: use podman")}
	tr := NewTr(testInfo(), caps, Policy{}, "run")

	if tr.Allow("restart") {
		t.Fatal("an unsupported flag must not be emitted")
	}
	var u *diag.Unsupported
	if !errors.As(tr.Err(), &u) {
		t.Fatalf("expected *diag.Unsupported, got %T", tr.Err())
	}
	if u.Flag != "restart" || u.Runtime != "container" {
		t.Errorf("error lost its context: %+v", u)
	}
	if !strings.Contains(u.Error(), "Nearest option: use podman") {
		t.Error("the alternative should reach the engineer")
	}
}

func TestFirstErrorWins(t *testing.T) {
	caps := Caps{"run.restart": No("first"), "run.privileged": No("second")}
	tr := NewTr(testInfo(), caps, Policy{}, "run")
	tr.Allow("restart")
	tr.Allow("privileged")

	// Reporting the first refusal keeps the message about what the engineer typed
	// first, rather than whichever flag the adapter happens to check last.
	if !strings.Contains(tr.Err().Error(), "first") {
		t.Errorf("expected the first refusal to be reported, got: %v", tr.Err())
	}
}

func TestRelaxDropsWithWarning(t *testing.T) {
	caps := Caps{"run.restart": No("no restart policy")}
	tr := NewTr(testInfo(), caps, Policy{Relax: true}, "run")

	if tr.Allow("restart") {
		t.Fatal("a dropped flag must not be emitted")
	}
	if tr.Err() != nil {
		t.Fatalf("--relax must not fail: %v", tr.Err())
	}
	if len(tr.Warnings()) != 1 || !strings.Contains(tr.Warnings()[0], "dropped") {
		t.Errorf("dropping must warn, got %v", tr.Warnings())
	}
}

func TestPerFlagPolicyOverridesStrictness(t *testing.T) {
	caps := Caps{"run.restart": No("no restart policy"), "run.privileged": No("no privileged")}
	policy := Policy{Flags: map[string]string{"restart": "drop"}}
	tr := NewTr(testInfo(), caps, policy, "run")

	tr.Allow("restart")
	if tr.Err() != nil {
		t.Fatalf("restart was relaxed for this flag: %v", tr.Err())
	}
	tr.Allow("privileged")
	if tr.Err() == nil {
		t.Fatal("privileged was not relaxed and must still fail")
	}
}

func TestLimitedDoesNotWarn(t *testing.T) {
	caps := Caps{"ps.format": Ltd("json|table only")}
	tr := NewTr(testInfo(), caps, Policy{}, "ps")
	if !tr.Allow("format") {
		t.Fatal("a limited flag is still emitted")
	}
	if len(tr.Warnings()) != 0 {
		t.Errorf("limited flags are validated by value, not warned about: %v", tr.Warnings())
	}
}

func TestDivergentWarnsButEmits(t *testing.T) {
	caps := Caps{"images.quiet": Div("prints references, not IDs")}
	tr := NewTr(testInfo(), caps, Policy{}, "images")
	if !tr.Allow("quiet") {
		t.Fatal("a divergent flag is still emitted")
	}
	if len(tr.Warnings()) != 1 {
		t.Fatalf("a divergence must be surfaced, got %v", tr.Warnings())
	}
}

func TestCapsWildcardLookup(t *testing.T) {
	caps := Caps{"*.privileged": No("no privileged anywhere")}
	if caps.Lookup("run", "privileged").Level != Unsupported {
		t.Error("a wildcard entry should apply to every command")
	}
	if caps.Lookup("exec", "privileged").Level != Unsupported {
		t.Error("a wildcard entry should apply to every command")
	}
}

func TestInvocationStringQuotesWhatNeedsIt(t *testing.T) {
	inv := Invocation{Bin: "docker", Args: []string{"run", "--rm", "alpine", "sh", "-c", "echo hi"}}
	want := "docker run --rm alpine sh -c 'echo hi'"
	if got := inv.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractVersion(t *testing.T) {
	cases := map[string]string{
		"Docker version 27.3.1, build ce12230":                      "27.3.1",
		"podman version 5.2.2":                                      "5.2.2",
		"container CLI version 1.2.2 (build: release, commit: 019)": "1.2.2",
		"nothing here": "",
	}
	for raw, want := range cases {
		if got := ExtractVersion(raw); got != want {
			t.Errorf("ExtractVersion(%q) = %q, want %q", raw, got, want)
		}
	}
}
