package temporal

import (
	"errors"
	"fmt"
)

// Temporal throws two kinds of error, and every error this package returns
// wraps one of them, which errors.Is tells apart: ErrRange where a value is
// out of range, and ErrType where one is missing or of the wrong kind.
var (
	ErrRange = errors.New("RangeError")
	ErrType  = errors.New("TypeError")
)

func rangeError(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrRange}, args...)...)
}

func typeError(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrType}, args...)...)
}
