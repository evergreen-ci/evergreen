package testutil

import "time"

// Constants for test timeouts.
const (
	TestTimeout         = 5 * time.Second
	RPCTestTimeout      = 30 * time.Second
	ProcessTestTimeout  = 15 * time.Second
	ManagerTestTimeout  = 5 * TestTimeout
	ExecutorTestTimeout = 5 * time.Second
	LongTestTimeout     = 100 * time.Second
)
