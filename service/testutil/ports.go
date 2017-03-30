package testutil

import "sync"

var (
	lastPort      = 8179
	lastPortMutex *sync.Mutex
)

func init() {
	lastPortMutex = &sync.Mutex{}
}

func NextPort() int {
	lastPortMutex.Lock()
	defer lastPortMutex.Unlock()

	lastPort += 2

	return lastPort
}
