// Package spec defines the runtime-neutral intermediate representation of a
// container operation.
//
// anyctr parses a Docker-convention command line exactly once, into one of the
// types below. Runtime adapters then translate that value into the argv of
// whichever CLI the engineer actually has installed. Nothing in this package
// knows about docker, podman or Apple's container.
//
// Conventions:
//   - Scalar flags are modelled as strings; the empty string means "unset".
//     Values are passed through verbatim (e.g. "512m", "1.5", "10").
//   - Repeatable flags are slices; nil means "unset".
//   - Field names match the canonical Docker long flag they come from, so
//     adapters and the capability matrix can be read side by side.
package spec

// Op is a single container operation, already parsed from the command line.
// Command reports the canonical anyctr command name ("run", "ps", ...), which
// is also the prefix used for capability lookups (e.g. "run.restart").
type Op interface {
	Command() string
}

// Run is `docker run`.
type Run struct {
	Image string
	Cmd   []string

	AddHost       []string
	CapAdd        []string
	CapDrop       []string
	Cidfile       string
	CPUShares     string
	CPUs          string
	Detach        bool
	Device        []string
	DNS           []string
	DNSOption     []string
	DNSSearch     []string
	Entrypoint    string
	EntrypointSet bool // distinguishes --entrypoint="" from an absent flag
	Env           []string
	EnvFile       []string
	GPUs          string
	Hostname      string
	Init          bool
	Interactive   bool
	IPC           string
	Label         []string
	LabelFile     []string
	LogDriver     string
	LogOpt        []string
	Memory        string
	MemorySwap    string
	Mount         []string
	Name          string
	Network       []string
	NetworkAlias  []string
	PID           string
	Platform      string
	Privileged    bool
	Publish       []string
	PublishAll    bool
	Pull          string
	ReadOnly      bool
	Restart       string
	Rm            bool
	SecurityOpt   []string
	ShmSize       string
	StopSignal    string
	StopTimeout   string
	Sysctl        []string
	Tmpfs         []string
	TTY           bool
	Ulimit        []string
	User          string
	Userns        string
	Volume        []string
	Workdir       string
}

func (*Run) Command() string { return "run" }

// Ps is `docker ps`.
type Ps struct {
	All     bool
	Filter  []string
	Format  string
	Last    string
	Latest  bool
	NoTrunc bool
	Quiet   bool
	Size    bool
}

func (*Ps) Command() string { return "ps" }

// Images is `docker images`.
type Images struct {
	Reference string // optional positional [REPOSITORY[:TAG]]

	All     bool
	Digests bool
	Filter  []string
	Format  string
	NoTrunc bool
	Quiet   bool
}

func (*Images) Command() string { return "images" }

// Build is `docker build`.
type Build struct {
	Context string

	AddHost   []string
	BuildArg  []string
	CacheFrom []string
	CacheTo   []string
	File      string
	Label     []string
	Network   string
	NoCache   bool
	Output    string
	Platform  string
	Progress  string
	Pull      bool
	Quiet     bool
	Secret    []string
	SSH       []string
	Tag       []string
	Target    string
}

func (*Build) Command() string { return "build" }

// Pull is `docker pull`.
type Pull struct {
	Reference string

	AllTags             bool
	DisableContentTrust bool
	Platform            string
	Quiet               bool
}

func (*Pull) Command() string { return "pull" }

// Push is `docker push`.
type Push struct {
	Reference string

	AllTags  bool
	Platform string
	Quiet    bool
}

func (*Push) Command() string { return "push" }

// Logs is `docker logs`.
type Logs struct {
	Container string

	Details    bool
	Follow     bool
	Since      string
	Tail       string
	Timestamps bool
	Until      string
}

func (*Logs) Command() string { return "logs" }

// Exec is `docker exec`.
type Exec struct {
	Container string
	Cmd       []string

	Detach      bool
	Env         []string
	EnvFile     []string
	Interactive bool
	Privileged  bool
	TTY         bool
	User        string
	Workdir     string
}

func (*Exec) Command() string { return "exec" }

// Stop is `docker stop`.
type Stop struct {
	Containers []string

	All    bool // not a Docker flag; accepted because most runtimes have it
	Signal string
	Time   string
}

func (*Stop) Command() string { return "stop" }

// Rm is `docker rm`.
type Rm struct {
	Containers []string

	All     bool // not a Docker flag; accepted because most runtimes have it
	Force   bool
	Link    bool
	Volumes bool
}

func (*Rm) Command() string { return "rm" }

// Rmi is `docker rmi`.
type Rmi struct {
	Images []string

	All     bool // not a Docker flag; accepted because most runtimes have it
	Force   bool
	NoPrune bool
}

func (*Rmi) Command() string { return "rmi" }

// Tag is `docker tag`.
type Tag struct {
	Source string
	Target string
}

func (*Tag) Command() string { return "tag" }

// Compose is `docker compose <args...>`. anyctr does not interpret compose
// files yet; it routes to whichever compose implementation the runtime ships.
type Compose struct {
	Args []string
}

func (*Compose) Command() string { return "compose" }
