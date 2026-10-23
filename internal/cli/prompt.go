package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// confirm asks a yes/no question on the controlling terminal.
//
// It reads from /dev/tty rather than stdin when stdin is not a terminal, so a
// question still reaches the engineer under `curl ... | sh`, where stdin is the
// script itself. When there is no terminal at all — CI, a pipeline — it returns
// an error so the caller can print instructions instead of blocking on a prompt
// nobody will ever answer.
func confirm(out io.Writer, question string, defaultYes bool) (bool, error) {
	in, closer, err := promptInput()
	if err != nil {
		return false, err
	}
	if closer != nil {
		defer closer.Close()
	}

	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(out, "%s %s ", question, suffix)

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false, errors.New("no answer")
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	case "":
		return defaultYes, nil
	}
	return false, nil
}

// promptInput returns something to read an answer from, preferring stdin when it
// is a terminal and falling back to the controlling terminal.
func promptInput() (io.Reader, io.Closer, error) {
	if isTerminal(os.Stdin) {
		return os.Stdin, nil, nil
	}
	tty, err := os.Open(ttyDevice)
	if err != nil {
		return nil, nil, errors.New("no terminal")
	}
	if !isTerminal(tty) {
		tty.Close()
		return nil, nil, errors.New("no terminal")
	}
	return tty, tty, nil
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
