package runtime

// Builder accumulates argv for one operation, consulting the capability model
// for every flag it is asked to emit.
//
// Flags that were not set are never consulted, so engineers only ever hear about
// flags they actually typed. Each method takes the *canonical* Docker flag name
// (used for the capability lookup and for error messages) and the ...As variants
// additionally take the token to emit, for runtimes that spell it differently —
// e.g. Docker's `--tail 5` is `-n 5` on Apple's container.
type Builder struct {
	tr *Tr

	// Joined emits `--flag=value` (Docker, podman). When false, flag and value
	// are separate tokens, which is what Swift ArgumentParser CLIs expect.
	Joined bool

	// Renames overrides the token emitted for a canonical flag, for runtimes in
	// an otherwise Docker-compatible family that spell one differently — Docker
	// renamed `docker stop --time` to `--timeout`, while podman kept `--time`.
	Renames map[string]string

	args []string
}

// NewBuilder starts an argv for the given translation.
func NewBuilder(tr *Tr, joined bool) *Builder {
	return &Builder{tr: tr, Joined: joined}
}

// Add appends raw tokens (subcommands, positionals).
func (b *Builder) Add(vals ...string) { b.args = append(b.args, vals...) }

// Args returns the accumulated argv.
func (b *Builder) Args() []string { return b.args }

// token is the flag spelling this runtime uses for a canonical flag.
func (b *Builder) token(canonical string) string {
	if t, ok := b.Renames[canonical]; ok {
		return t
	}
	return "--" + canonical
}

// Bool emits the runtime's spelling of the flag when on.
func (b *Builder) Bool(canonical string, on bool) {
	b.BoolAs(canonical, b.token(canonical), on)
}

// BoolAs emits the given token when on.
func (b *Builder) BoolAs(canonical, emit string, on bool) {
	if on && b.tr.Allow(canonical) {
		b.args = append(b.args, emit)
	}
}

// Str emits --canonical=value (or `--canonical value`) when value is set.
func (b *Builder) Str(canonical, value string) {
	b.StrAs(canonical, b.token(canonical), value)
}

// StrAs emits the given token with value when value is set.
func (b *Builder) StrAs(canonical, emit, value string) {
	if value == "" || !b.tr.Allow(canonical) {
		return
	}
	b.emit(emit, value)
}

// Rep emits the flag once per value.
func (b *Builder) Rep(canonical string, values []string) {
	b.RepAs(canonical, b.token(canonical), values)
}

// RepAs emits the given token once per value.
func (b *Builder) RepAs(canonical, emit string, values []string) {
	if len(values) == 0 || !b.tr.Allow(canonical) {
		return
	}
	for _, v := range values {
		b.emit(emit, v)
	}
}

// Raw emits the given tokens verbatim if the canonical flag is allowed. It exists
// for flags whose value may legitimately be empty, such as Docker's
// `--entrypoint=""` ("clear the image entrypoint").
func (b *Builder) Raw(canonical string, tokens ...string) {
	if b.tr.Allow(canonical) {
		b.args = append(b.args, tokens...)
	}
}

func (b *Builder) emit(flag, value string) {
	if b.Joined {
		b.args = append(b.args, flag+"="+value)
		return
	}
	b.args = append(b.args, flag, value)
}
