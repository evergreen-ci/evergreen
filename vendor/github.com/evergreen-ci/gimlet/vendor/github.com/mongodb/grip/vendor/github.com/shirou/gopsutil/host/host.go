package host

import (
	"encoding/json"

	"github.com/shirou/gopsutil/internal/common"
)

var invoke common.Invoker = common.Invoke{}

// A HostInfoStat describes the host status.
// This is not in the psutil but it useful.
type InfoStat struct {
	Hostname             string `json:"hostname" bson:"hostname,omitempty"`
	Uptime               uint64 `json:"uptime" bson:"uptime,omitempty"`
	BootTime             uint64 `json:"bootTime" bson:"bootTime,omitempty"`
	Procs                uint64 `json:"procs" bson:"procs,omitempty"`                     // number of processes
	OS                   string `json:"os" bson:"os,omitempty"`                           // ex: freebsd, linux
	Platform             string `json:"platform" bson:"platform,omitempty"`               // ex: ubuntu, linuxmint
	PlatformFamily       string `json:"platformFamily" bson:"platformFamily,omitempty"`   // ex: debian, rhel
	PlatformVersion      string `json:"platformVersion" bson:"platformVersion,omitempty"` // version of the complete OS
	KernelVersion        string `json:"kernelVersion" bson:"kernelVersion,omitempty"`     // version of the OS kernel (if available)
	KernelArch           string `json:"kernelArch" bson:"kernelArch,omitempty"`           // native cpu architecture queried at runtime, as returned by `uname -m` or empty string in case of error
	VirtualizationSystem string `json:"virtualizationSystem" bson:"virtualizationSystem,omitempty"`
	VirtualizationRole   string `json:"virtualizationRole" bson:"virtualizationRole,omitempty"` // guest or host
	HostID               string `json:"hostid" bson:"hostid,omitempty"`                         // ex: uuid
}

type UserStat struct {
	User     string `json:"user" bson:"user,omitempty"`
	Terminal string `json:"terminal" bson:"terminal,omitempty"`
	Host     string `json:"host" bson:"host,omitempty"`
	Started  int    `json:"started" bson:"started,omitempty"`
}

type TemperatureStat struct {
	SensorKey   string  `json:"sensorKey" bson:"sensorKey,omitempty"`
	Temperature float64 `json:"sensorTemperature" bson:"sensorTemperature,omitempty"`
}

func (h InfoStat) String() string {
	s, _ := json.Marshal(h)
	return string(s)
}

func (u UserStat) String() string {
	s, _ := json.Marshal(u)
	return string(s)
}

func (t TemperatureStat) String() string {
	s, _ := json.Marshal(t)
	return string(s)
}
