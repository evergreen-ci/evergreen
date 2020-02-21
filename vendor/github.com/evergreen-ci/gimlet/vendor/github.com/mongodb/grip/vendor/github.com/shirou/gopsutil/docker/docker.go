package docker

import (
	"encoding/json"
	"errors"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/internal/common"
)

var ErrDockerNotAvailable = errors.New("docker not available")
var ErrCgroupNotAvailable = errors.New("cgroup not available")

var invoke common.Invoker = common.Invoke{}

const nanoseconds = 1e9

type CgroupCPUStat struct {
	cpu.TimesStat
	Usage float64
}

type CgroupMemStat struct {
	ContainerID             string `json:"containerID" bson:"containerID,omitempty"`
	Cache                   uint64 `json:"cache" bson:"cache,omitempty"`
	RSS                     uint64 `json:"rss" bson:"rss,omitempty"`
	RSSHuge                 uint64 `json:"rssHuge" bson:"rssHuge,omitempty"`
	MappedFile              uint64 `json:"mappedFile" bson:"mappedFile,omitempty"`
	Pgpgin                  uint64 `json:"pgpgin" bson:"pgpgin,omitempty"`
	Pgpgout                 uint64 `json:"pgpgout" bson:"pgpgout,omitempty"`
	Pgfault                 uint64 `json:"pgfault" bson:"pgfault,omitempty"`
	Pgmajfault              uint64 `json:"pgmajfault" bson:"pgmajfault,omitempty"`
	InactiveAnon            uint64 `json:"inactiveAnon" bson:"inactiveAnon,omitempty"`
	ActiveAnon              uint64 `json:"activeAnon" bson:"activeAnon,omitempty"`
	InactiveFile            uint64 `json:"inactiveFile" bson:"inactiveFile,omitempty"`
	ActiveFile              uint64 `json:"activeFile" bson:"activeFile,omitempty"`
	Unevictable             uint64 `json:"unevictable" bson:"unevictable,omitempty"`
	HierarchicalMemoryLimit uint64 `json:"hierarchicalMemoryLimit" bson:"hierarchicalMemoryLimit,omitempty"`
	TotalCache              uint64 `json:"totalCache" bson:"totalCache,omitempty"`
	TotalRSS                uint64 `json:"totalRss" bson:"totalRss,omitempty"`
	TotalRSSHuge            uint64 `json:"totalRssHuge" bson:"totalRssHuge,omitempty"`
	TotalMappedFile         uint64 `json:"totalMappedFile" bson:"totalMappedFile,omitempty"`
	TotalPgpgIn             uint64 `json:"totalPgpgin" bson:"totalPgpgin,omitempty"`
	TotalPgpgOut            uint64 `json:"totalPgpgout" bson:"totalPgpgout,omitempty"`
	TotalPgFault            uint64 `json:"totalPgfault" bson:"totalPgfault,omitempty"`
	TotalPgMajFault         uint64 `json:"totalPgmajfault" bson:"totalPgmajfault,omitempty"`
	TotalInactiveAnon       uint64 `json:"totalInactiveAnon" bson:"totalInactiveAnon,omitempty"`
	TotalActiveAnon         uint64 `json:"totalActiveAnon" bson:"totalActiveAnon,omitempty"`
	TotalInactiveFile       uint64 `json:"totalInactiveFile" bson:"totalInactiveFile,omitempty"`
	TotalActiveFile         uint64 `json:"totalActiveFile" bson:"totalActiveFile,omitempty"`
	TotalUnevictable        uint64 `json:"totalUnevictable" bson:"totalUnevictable,omitempty"`
	MemUsageInBytes         uint64 `json:"memUsageInBytes" bson:"memUsageInBytes,omitempty"`
	MemMaxUsageInBytes      uint64 `json:"memMaxUsageInBytes" bson:"memMaxUsageInBytes,omitempty"`
	MemLimitInBytes         uint64 `json:"memoryLimitInBbytes" bson:"memoryLimitInBbytes,omitempty"`
	MemFailCnt              uint64 `json:"memoryFailcnt" bson:"memoryFailcnt,omitempty"`
}

func (m CgroupMemStat) String() string {
	s, _ := json.Marshal(m)
	return string(s)
}

type CgroupDockerStat struct {
	ContainerID string `json:"containerID" bson:"containerID,omitempty"`
	Name        string `json:"name" bson:"name,omitempty"`
	Image       string `json:"image" bson:"image,omitempty"`
	Status      string `json:"status" bson:"status,omitempty"`
	Running     bool   `json:"running" bson:"running,omitempty"`
}

func (c CgroupDockerStat) String() string {
	s, _ := json.Marshal(c)
	return string(s)
}
