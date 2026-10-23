//go:build !windows

package cli

// ttyDevice is the controlling terminal, used when stdin is a pipe.
const ttyDevice = "/dev/tty"
