package detect

import "errors"

func asError(err error, target any) bool { return errors.As(err, target) }
