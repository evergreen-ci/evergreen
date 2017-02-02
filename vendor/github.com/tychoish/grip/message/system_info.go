package message

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
	"github.com/tychoish/grip/level"
)

// SystemInfo is a type that implements message.Composer but also
// collects system-wide resource utilization statistics about memory,
// CPU, and network use, along with an optional message.
type SystemInfo struct {
	Message  string                `json:"message,omitempty" bson:"message,omitempty"`
	CPU      cpu.TimesStat         `json:"cpu,omitempty" bson:"cpu,omitempty"`
	NumCPU   int                   `json:"num_cpus,omitempty" bson:"num_cpus,omitempty"`
	VMStat   mem.VirtualMemoryStat `json:"vmstat,omitempty" bson:"vmstat,omitempty"`
	NetStat  net.IOCountersStat    `json:"netstat,omitempty" bson:"netstat,omitempty"`
	Errors   []string              `json:"errors,omitempty" bson:"errors,omitempty"`
	Base     `json:"metadata,omitempty" bson:"metadata,omitempty"`
	loggable bool
}

// CollectSystemInfo returns a populated SystemInfo object,
// without a message.
func CollectSystemInfo() Composer {
	return NewSystemInfo(level.Trace, "")
}

// MakeSystemInfo builds a populated SystemInfo object with the
// specified message.
func MakeSystemInfo(message string) Composer {
	return NewSystemInfo(level.Info, message)
}

// NewSystemInfo returns a fully configured and populated SystemInfo
// object.
func NewSystemInfo(priority level.Priority, message string) Composer {
	s := &SystemInfo{
		Message: message,
		NumCPU:  runtime.NumCPU(),
	}

	if err := s.SetPriority(priority); err != nil {
		s.Errors = append(s.Errors, err.Error())
		return s
	}

	s.loggable = true

	times, err := cpu.Times(false)
	s.saveError(err)
	if err == nil {
		// since we're not storing per-core information,
		// there's only one thing we care about in this struct
		s.CPU = times[0]
	}

	vmstat, err := mem.VirtualMemory()
	s.saveError(err)
	if err != nil {
		s.VMStat = *vmstat
	}

	netstat, err := net.IOCounters(false)
	s.saveError(err)
	if err == nil {
		s.NetStat = netstat[0]
	}

	return s
}

func (s *SystemInfo) Loggable() bool   { return s.loggable }
func (s *SystemInfo) Raw() interface{} { _ = s.Collect(); return s }
func (s *SystemInfo) String() string {
	data, err := json.MarshalIndent(s, "  ", " ")
	if err != nil {
		return s.Message
	}

	return fmt.Sprintf("%s:\n%s", s.Message, string(data))
}

func (s *SystemInfo) saveError(err error) {
	if shouldSaveError(err) {
		s.Errors = append(s.Errors, err.Error())
	}
}

// helper function
func shouldSaveError(err error) bool {
	return err != nil && err.Error() != "not implemented yet"
}
