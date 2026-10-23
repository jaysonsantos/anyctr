package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// explain runs anyctr in --explain mode and returns the translated command line.
// Because --explain never executes anything, these tests run identically on
// macOS, Linux and Windows, with or without any runtime installed.
func explain(t *testing.T, args ...string) (command, warnings string, err error) {
	t.Helper()
	t.Setenv("ANYCTR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	t.Setenv("ANYCTR_RUNTIME", "")
	t.Setenv("ANYCTR_RELAX", "")

	var out, errBuf bytes.Buffer
	a := &app{out: &out, err: &errBuf}
	root := newRootCmd("test", a)
	root.SetArgs(append([]string{"--explain"}, args...))
	root.SetOut(&out)
	root.SetErr(&errBuf)

	err = root.Execute()
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "command: ") {
			command = normalizeBinary(strings.TrimPrefix(line, "command: "))
		}
	}
	return command, errBuf.String(), err
}

// normalizeBinary reduces the resolved binary path to its name, so expectations
// hold whether or not the runtime happens to be installed on the test machine.
func normalizeBinary(command string) string {
	bin, rest, found := strings.Cut(command, " ")
	bin = filepath.Base(bin)
	if !found {
		return bin
	}
	return bin + " " + rest
}

func TestTranslatesRunAcrossRuntimes(t *testing.T) {
	args := []string{"run", "-d", "-it", "--rm", "-p", "8080:80", "-e", "FOO=bar",
		"--name", "web", "-v", "/src:/app", "-w", "/app", "nginx:alpine", "sh", "-c", "echo hi"}

	cases := []struct {
		runtime string
		want    string
	}{{
		runtime: "docker",
		want: "docker run --detach --env=FOO=bar --interactive --name=web " +
			"--publish=8080:80 --rm --tty --volume=/src:/app --workdir=/app " +
			"nginx:alpine sh -c 'echo hi'",
	}, {
		runtime: "podman",
		want: "podman run --detach --env=FOO=bar --interactive --name=web " +
			"--publish=8080:80 --rm --tty --volume=/src:/app --workdir=/app " +
			"nginx:alpine sh -c 'echo hi'",
	}, {
		runtime: "container",
		want: "container run --detach --env FOO=bar --interactive --name web " +
			"--publish 8080:80 --rm --tty --volume /src:/app --workdir /app " +
			"nginx:alpine sh -c 'echo hi'",
	}}

	for _, tc := range cases {
		t.Run(tc.runtime, func(t *testing.T) {
			got, _, err := explain(t, append([]string{"--runtime=" + tc.runtime}, args...)...)
			if err != nil {
				t.Fatalf("translation failed: %v", err)
			}
			if got != tc.want {
				t.Errorf("translated command\n  got:  %s\n  want: %s", got, tc.want)
			}
		})
	}
}

// TestCommandNameMapping covers the subcommand renames that make Apple's CLI
// incompatible with Docker's even when the flags line up.
func TestCommandNameMapping(t *testing.T) {
	cases := []struct{ name, args, want string }{
		{"ps", "ps -a", "container list --all"},
		{"images", "images", "container image list"},
		{"pull", "pull alpine", "container image pull alpine"},
		{"push", "push me/app:1", "container image push me/app:1"},
		{"rm", "rm -f web", "container delete --force web"},
		{"rmi", "rmi alpine", "container image delete alpine"},
		{"tag", "tag a:1 b:2", "container image tag a:1 b:2"},
		{"logs tail", "logs --tail 20 -f web", "container logs --follow -n 20 web"},
		{"logs tail all", "logs --tail all web", "container logs web"},
		{"build", "build -t app:dev .", "container build --tag app:dev ."},
		{"exec", "exec -it web sh", "container exec --interactive --tty web sh"},
		{"stop", "stop -t 3 web", "container stop --time 3 web"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := explain(t, append([]string{"--runtime=container"}, strings.Fields(tc.args)...)...)
			if err != nil {
				t.Fatalf("translation failed: %v", err)
			}
			if got != tc.want {
				t.Errorf("translated command\n  got:  %s\n  want: %s", got, tc.want)
			}
		})
	}
}

// TestStrictRefusals is the heart of the gateway: a flag with no faithful
// translation must fail, and the message must name the flag and the runtime.
func TestStrictRefusals(t *testing.T) {
	cases := []struct {
		name string
		args []string
		flag string
	}{
		{"restart policy", []string{"run", "--restart=always", "nginx"}, "--restart"},
		{"privileged", []string{"run", "--privileged", "nginx"}, "--privileged"},
		{"host network", []string{"run", "--network=host", "nginx"}, "--network"},
		{"fractional cpus", []string{"run", "--cpus=1.5", "nginx"}, "--cpus"},
		{"go template format", []string{"ps", "--format", "{{.ID}}"}, "--format"},
		{"ps filter", []string{"ps", "--filter", "status=running"}, "--filter"},
		{"log timestamps", []string{"logs", "-t", "web"}, "--timestamps"},
		{"build cache-from", []string{"build", "--cache-from", "type=gha", "."}, "--cache-from"},
		{"images reference", []string{"images", "nginx"}, "REPOSITORY[:TAG]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := explain(t, append([]string{"--runtime=container"}, tc.args...)...)
			if err == nil {
				t.Fatalf("expected a refusal for %s, got none", tc.flag)
			}
			if !isUnsupported(err) {
				t.Fatalf("expected an unsupported-flag error, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), tc.flag) {
				t.Errorf("error should name %s, got:\n%v", tc.flag, err)
			}
			if !strings.Contains(err.Error(), "container") {
				t.Errorf("error should name the runtime, got:\n%v", err)
			}
		})
	}
}

// TestRelaxDropsInsteadOfFailing checks the escape hatch: the flag is dropped,
// the command still runs, and the engineer is told on stderr.
func TestRelaxDropsInsteadOfFailing(t *testing.T) {
	t.Setenv("ANYCTR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	var out, errBuf bytes.Buffer
	a := &app{out: &out, err: &errBuf}
	root := newRootCmd("test", a)
	root.SetArgs([]string{"--explain", "--relax", "--runtime=container", "run", "--restart=always", "-d", "nginx"})
	if err := root.Execute(); err != nil {
		t.Fatalf("--relax should not fail: %v", err)
	}
	if !strings.Contains(out.String(), "container run --detach nginx") {
		t.Errorf("unsupported flag should have been dropped, got:\n%s", out.String())
	}
	if !strings.Contains(errBuf.String(), "--restart is not supported") {
		t.Errorf("dropping a flag must warn, got:\n%s", errBuf.String())
	}
}

// TestDivergentFlagWarns covers flags that do translate but mean something
// slightly different — silence there is how engineers get surprised later.
func TestDivergentFlagWarns(t *testing.T) {
	got, warnings, err := explain(t, "--runtime=container", "images", "-q")
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	if got != "container image list --quiet" {
		t.Errorf("unexpected translation: %s", got)
	}
	if !strings.Contains(warnings, "image references") {
		t.Errorf("expected a divergence warning about -q, got:\n%s", warnings)
	}
}

// TestPerFlagPolicy checks that strictness can be relaxed for one flag only,
// which is how a team adopts a runtime that is missing something they don't need.
func TestPerFlagPolicy(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := writeFile(cfg, "strict = true\n\n[flags]\n  restart = \"drop\"\n"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANYCTR_CONFIG", cfg)

	var out, errBuf bytes.Buffer
	a := &app{out: &out, err: &errBuf}
	root := newRootCmd("test", a)
	root.SetArgs([]string{"--explain", "--runtime=container", "run", "--restart=always", "--privileged", "nginx"})
	err := root.Execute()
	if err == nil {
		t.Fatal("--privileged should still fail when only restart is relaxed")
	}
	if !strings.Contains(err.Error(), "--privileged") {
		t.Errorf("expected --privileged to be the refusal, got:\n%v", err)
	}
}

// TestFlagsAfterImageBelongToContainer is the parsing rule that makes Docker's
// convention work: anyctr must not steal the container's own flags.
func TestFlagsAfterImageBelongToContainer(t *testing.T) {
	got, _, err := explain(t, "--runtime=docker", "run", "--rm", "alpine", "sh", "-c", "ls -la")
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	want := "docker run --rm alpine sh -c 'ls -la'"
	if got != want {
		t.Errorf("translated command\n  got:  %s\n  want: %s", got, want)
	}

	// -e after the image is the container's flag, not anyctr's --env.
	got, _, err = explain(t, "--runtime=docker", "run", "alpine", "echo", "-e", "x")
	if err != nil {
		t.Fatalf("translation failed: %v", err)
	}
	if want := "docker run alpine echo -e x"; got != want {
		t.Errorf("translated command\n  got:  %s\n  want: %s", got, want)
	}
}

// TestUnknownFlagPointsAtRaw keeps the escape hatch discoverable.
func TestUnknownFlagPointsAtRaw(t *testing.T) {
	_, _, err := explain(t, "--runtime=docker", "run", "--cgroupns=host", "nginx")
	if err == nil {
		t.Fatal("expected an error for an unmodelled flag")
	}
	if !strings.Contains(err.Error(), "anyctr raw run") {
		t.Errorf("error should point at `anyctr raw`, got:\n%v", err)
	}
}

// TestComposeRouting checks the compose router, including the runtime that has no
// Compose at all.
func TestComposeRouting(t *testing.T) {
	got, _, err := explain(t, "--runtime=podman", "compose", "up", "-d")
	if err != nil {
		t.Fatalf("podman compose should route: %v", err)
	}
	if want := "podman compose up -d"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	_, _, err = explain(t, "--runtime=container", "compose", "up")
	if err == nil {
		t.Fatal("Apple's container has no compose; expected a refusal")
	}
	if !strings.Contains(err.Error(), "no Compose implementation") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestRawPassesThrough checks that the escape hatch translates nothing.
func TestRawPassesThrough(t *testing.T) {
	got, _, err := explain(t, "--runtime=container", "raw", "run", "--rosetta", "--rm", "alpine")
	if err != nil {
		t.Fatalf("raw should never refuse: %v", err)
	}
	if want := "container run --rosetta --rm alpine"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestUnknownRuntime checks the error for a runtime anyctr does not know.
func TestUnknownRuntime(t *testing.T) {
	_, _, err := explain(t, "--runtime=nerdctl", "ps")
	if err == nil {
		t.Fatal("expected an error for an unknown runtime")
	}
	if !strings.Contains(err.Error(), "unknown runtime") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestCapabilityMatrixCoversEveryFlag guards the docs against drift: every flag
// the CLI accepts must appear in the generated matrix.
func TestCapabilityMatrixCoversEveryFlag(t *testing.T) {
	t.Setenv("ANYCTR_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	var out, errBuf bytes.Buffer
	a := &app{out: &out, err: &errBuf}
	root := newRootCmd("test", a)
	rows := buildMatrix(root, gatewayCommands)
	if len(rows) < 60 {
		t.Fatalf("capability matrix looks too small: %d rows", len(rows))
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.Command+"."+r.Flag] = true
		if len(r.Cells) != 3 {
			t.Fatalf("row %s.%s covers %d runtimes, want 3", r.Command, r.Flag, len(r.Cells))
		}
	}
	for _, key := range []string{"run.restart", "run.publish", "ps.format", "build.tag", "logs.tail"} {
		if !seen[key] {
			t.Errorf("capability matrix is missing %s", key)
		}
	}
}
