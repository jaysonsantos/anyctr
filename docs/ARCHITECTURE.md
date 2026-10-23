# Architecture

anyctr parses a Docker-convention command line **once**, into a runtime-neutral
value, then asks the selected runtime's adapter to render that value as an argv.
Nothing else in the program knows about docker, podman, or Apple's container.

```
                    internal/cli (cobra)
  argv ──────────►  Docker's flag surface, parsed once
                              │
                              ▼
                    internal/spec
                    *spec.Run, *spec.Ps, … — the neutral IR
                              │
        internal/detect ──────┤ which runtime? (flag > env > config > PATH)
                              ▼
                    internal/runtime/<adapter>
                    Translate(op, *runtime.Tr) → runtime.Invocation
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
       --explain: print                internal/procexec
       nothing executed                exec, forward signals, propagate status
```

The split that matters: **adapters never execute anything.** They return an
`Invocation` (binary + argv + terminal expectations), which the caller either runs
or prints. That is why `--explain` exists, why the entire translation matrix is
testable on any OS with no runtime installed, and why CI can verify Apple-container
translations on Linux.

## Packages

| package | responsibility |
| --- | --- |
| `cmd/anyctr` | entry point; injects the version, returns the exit status |
| `internal/cli` | the cobra command tree — Docker's flag surface, and the single `gateway()` path every command takes |
| `internal/spec` | the neutral IR: one struct per operation, field names matching Docker's long flags |
| `internal/runtime` | the `Adapter` contract, the capability model, the argv `Builder`, and `Tr` (translation state) |
| `internal/runtime/dockerlike` | shared translation for CLIs that already speak Docker's argv (docker, podman) |
| `internal/runtime/{docker,podman,applecontainer}` | per-runtime adapters: capabilities plus whatever is genuinely different |
| `internal/detect` | finds installed runtimes, probes versions, applies selection precedence |
| `internal/config` | the optional TOML config |
| `internal/procexec` | runs the child, forwards signals, propagates the exit code |
| `internal/diag` | user-facing error types, including the unsupported-flag message |

## The capability model

Every flag anyctr emits passes through `Tr.Allow(flag)`, which consults the
adapter's `Caps` map. Four levels:

| level | constructor | behaviour |
| --- | --- | --- |
| supported | *(default for unlisted flags)* | emit, silently |
| limited | `runtime.Ltd(note)` | emit; the adapter validates the value and rejects what it cannot honour |
| divergent | `runtime.Div(note)` | emit, and warn on stderr — the flag works but means something slightly different |
| unsupported | `runtime.No(reason, alts…)` | fail with a message naming the flag, runtime, reason, and alternatives; drop with a warning under `--relax` |

Absent keys default to *supported*, so an adapter only enumerates its exceptions.
Keys are `"<command>.<flag>"` (`"run.restart"`), with `"*.<flag>"` for gaps that
apply to every command.

Two consequences worth knowing:

- **`Allow` is only consulted for flags the engineer actually set.** Adapters call
  it unconditionally at the end of each translation — the `Builder` short-circuits
  on empty values — so nobody hears about a flag they didn't type.
- **The first refusal wins.** Later errors are discarded so the message is about
  the first thing that was actually wrong, while *all* warnings accumulate.

The docs are generated from this model plus the cobra tree
(`anyctr runtime capabilities --all --markdown`), so the flags anyctr documents are
exactly the flags anyctr accepts. `make docs-check` fails the build on drift.

## Strictness

Refusing is the default, and it is the point. A gateway that drops `--restart`
because the target runtime lacks it has silently changed what was asked for, and
the engineer finds out in production. Escape hatches, narrowest first:

```
--relax                          this invocation
flags.<flag> = "drop"            one flag, permanently (config)
strict = false                   everything (config)
anyctr raw …                     no translation at all
```

## Adding a runtime

1. Create `internal/runtime/<name>/` implementing `runtime.Adapter`.
   If the CLI is Docker-compatible, delegate to `dockerlike.Translate` and supply
   only `Caps` — that is all the podman adapter is.
2. Read the CLI's real `--help` output and encode the gaps in `Caps`. Do not infer
   support: every entry in the Apple adapter came from `container --help` on 1.2.2.
3. Register it in `detect.Adapters()`, and add it to `DefaultPriority` if it should
   be auto-selected.
4. Add translation cases to `internal/cli/cli_test.go` — they run via `--explain`,
   so no installation is needed.
5. `make docs` to refresh the matrix.

`Builder` handles the two argv dialects: `Joined: true` emits `--flag=value`
(docker, podman), `false` emits `--flag value` (Swift ArgumentParser CLIs like
Apple's). Use the `…As` variants when the target spells a flag differently —
Docker's `--tail 5` is Apple's `-n 5`.

## Adding a command

1. Add a struct to `internal/spec` with a `Command()` method. Name fields after
   Docker's long flags.
2. Add the cobra command in `internal/cli`, filling that struct and calling
   `a.gateway(s)`. Commands that take a container command (`run`, `exec`) must call
   `dockerStyle(cmd)` so flags stop at the first positional — otherwise anyctr
   steals the container's own flags.
3. Handle the new type in each adapter's `Translate` switch.
4. Add the name to `gatewayCommands` in `internal/cli/runtimecmd.go` so it appears
   in the capability matrix.

## Testing

Three tiers, because no machine has all four runtimes and one of them cannot run in
hosted CI at all.

**Tier 1 — translation (`internal/cli/cli_test.go`, no build tag).** Drives the real
cobra tree in `--explain` mode and asserts the exact argv. Needs no runtime, so the
Apple-container matrix is verified on Linux CI. This is also where strictness,
`--relax`, per-flag policy and the parsing rules are pinned.

**Tier 2 — conformance (`internal/cli/conformance_test.go`, `-tags conformance`).**
For every runtime installed, every flag of every gateway command is translated and
checked against that CLI's own `--help`:

- a flag the matrix marks supported must appear in the help for the subcommand
  anyctr actually invokes — including renames, since the check uses the *emitted*
  token (`-n`), not the canonical one (`--tail`);
- a flag the matrix refuses must **not** appear. This is the half that pays for
  itself: the day Apple ships `--restart`, the refusal becomes a lie and only this
  test notices.

The subcommand to ask for help is derived from the translation itself, so
`anyctr images` checks `container image list --help` without a second mapping table
to keep in sync. Only `--help` is invoked — no container starts, no engine needs to
be running.

**Tier 3 — integration (`internal/cli/integration_test.go`, `-tags integration`).**
Builds the binary and walks the real path: pull, run, ps, exec, logs, stop, rm,
build, tag, images, exit-code propagation, and Compose where it exists. Because it
runs the compiled binary, it also covers `procexec` and runtime detection. Every
test cleans up after itself.

`internal/procexec` additionally re-executes the test binary as a fake runtime
(`TestMain` + an env var), which tests argv forwarding and exit-code propagation on
every OS without shipping a shell script Windows cannot run.

## Notable decisions

**Cobra, and Docker's parsing quirks with it.** Cobra/pflag is what the Docker CLI
itself uses, so the flag surface behaves the way engineers expect. Two quirks are
reproduced deliberately: `SetInterspersed(false)` on `run`/`exec`, so
`anyctr run alpine sh -c 'ls -la'` passes `-c` to the container; and `-h` is spent
on `--hostname` for `run`, as Docker does, which requires pre-registering a `help`
flag without the shorthand.

**Passthrough commands parse their own globals.** `compose` and `raw` set
`DisableFlagParsing` so `-f compose.yaml` reaches the provider untouched. The cost
is that root's persistent flags arrive as plain arguments, so `consumeGlobals`
recognises them explicitly while they lead the argument list.

**Unmodelled flags are rejected, not forwarded.** anyctr cannot promise anything
about a flag it does not model, so it errors and points at `anyctr raw`. Silently
forwarding would make the strictness guarantee meaningless.

**Compose is routed, not interpreted.** Each adapter names its provider —
`docker compose`, `podman compose`, or `container-compose` for Apple's container,
which ships none — and `internal/cli/compose.go` checks that the provider is
actually there before running it, because a missing provider produces a far worse
error than anyctr's own. Arguments are forwarded verbatim, so compose files stay
the provider's business.

**No daemon probing.** Availability means "the CLI exists and reports a version".
Checking whether an engine is *running* is slow, racy, and the runtime's own error
messages are better than anything anyctr could synthesise.

**Apple's container cannot be tested end-to-end in hosted CI.** GitHub's macOS
runners are M1 VMs, and Apple's Virtualization Framework does not offer nested
virtualization there; the runner-images maintainers closed the request to add
apple/container for exactly that reason. Tier 1 covers its translation everywhere,
tier 2 may work on hosted runners because it never starts a VM, and tier 3 needs a
self-hosted Apple-silicon runner.

**Exit codes.** anyctr's own failures exit `125`, following Docker's convention;
otherwise the child's exit status is propagated unchanged, so scripts can tell
"the gateway refused" from "the container failed".
