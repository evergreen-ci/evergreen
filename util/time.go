package util

import (
	"time"
)

// ZeroTime represents 0 in epoch time
var ZeroTime time.Time = time.Unix(0, 0)

// TODO: rely on Go time.Time 0 instead
