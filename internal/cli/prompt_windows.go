//go:build windows

package cli

// ttyDevice is the console input device, the Windows equivalent of /dev/tty.
const ttyDevice = "CONIN$"
