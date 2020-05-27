// +build linux

package quota // import "github.com/docker/docker/daemon/graphdriver/quota"

import "sync"

// Quota limit params - currently we only control blocks hard limit
type Quota struct {
	Size uint64
}

// Control - Context to be used by storage driver (e.g. overlay)
// who wants to apply project quotas to container dirs
type Control struct {
	backingFsBlockDev string
	sync.RWMutex      // protect nextProjectID and quotas map
	nextProjectID     uint32
	quotas            map[string]uint32
}
