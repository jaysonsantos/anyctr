//go:build integration

package cli

import "errors"

func asExitError(err error, target any) bool { return errors.As(err, target) }
