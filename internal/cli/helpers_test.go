package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// splitBinary separates the leading binary token of a translated command from
// its arguments. The binary is shell-quoted when it contains spaces, which is
// the normal case on Windows ("C:\Program Files\Docker\docker.exe").
func splitBinary(command string) (bin, args string) {
	if strings.HasPrefix(command, "'") {
		if i := strings.Index(command[1:], "'"); i >= 0 {
			return command[1 : i+1], strings.TrimPrefix(command[i+2:], " ")
		}
	}
	if i := strings.IndexByte(command, ' '); i >= 0 {
		return command[:i], command[i+1:]
	}
	return command, ""
}

// normalizeBinary reduces a translated command's binary to its bare name, so
// expectations hold whether or not the runtime is installed, and on Windows
// where it resolves to a quoted path ending in .exe.
func normalizeBinary(command string) string {
	bin, args := splitBinary(command)
	name := strings.TrimSuffix(filepath.Base(bin), ".exe")
	if args == "" {
		return name
	}
	return name + " " + args
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
