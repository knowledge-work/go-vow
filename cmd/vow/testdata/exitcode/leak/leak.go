package leak

import "errors"

// vow:define @Sentinel
var ErrLeak = errors.New("leak")

func Leak() error { return ErrLeak }
