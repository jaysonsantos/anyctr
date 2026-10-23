// Command anyctr is a gateway for container runtime CLIs: it speaks Docker's
// convention and translates it for whichever runtime an engineer has installed.
package main

import (
	"os"

	"github.com/pedrospdc/anyctr/internal/cli"
)

// version is set at build time: -ldflags "-X main.version=$(git describe --tags)".
var version = "dev"

func main() {
	os.Exit(cli.Execute(version))
}
