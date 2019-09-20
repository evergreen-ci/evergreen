package testutil

import "time"

const (
	TestTimeout        = 5 * time.Second
	RPCTestTimeout     = 30 * time.Second
	ProcessTestTimeout = 15 * time.Second
	ManagerTestTimeout = 5 * TestTimeout
	LongTestTimeout    = 100 * time.Second
)
