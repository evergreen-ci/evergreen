package message

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

// SystemInfo is a type that implements message.Composer but also
// collects system-wide resource utilization statistics about memory,
// CPU, and network use, along with an optional message.
type SystemInfo struct {
	Message    string                `json:"message,omitempty" bson:"message,omitempty"`
	CPU        cpu.TimesStat         `json:"cpu,omitempty" bson:"cpu,omitempty"`
	NumCPU     int                   `json:"num_cpus,omitempty" bson:"num_cpus,omitempty"`
	VMStat     mem.VirtualMemoryStat `json:"vmstat,omitempty" bson:"vmstat,omitempty"`
	NetStat    net.IOCountersStat    `json:"netstat,omitempty" bson:"netstat,omitempty"`
	Partitions []disk.PartitionStat  `json:"partitions,omitempty" bson:"partitions,omitempty"`
	Usage      []disk.UsageStat      `json:"usage,omitempty" bson:"usage,omitempty"`
	IOStat     []disk.IOCountersStat `json:"iostat,omitempty" bson:"iostat,omitempty"`
	Errors     []string              `json:"errors,omitempty" bson:"errors,omitempty"`
	Base       `json:"metadata,omitempty" bson:"metadata,omitempty"`
	loggable   bool
	rendered   string
}

// CollectSystemInfo returns a populated SystemInfo object,
// without a message.
func CollectSystemInfo() Composer {
	info, _ := NewSystemInfo(level.Trace, "")
	return info
}

// MakeSystemInfo builds a populated SystemInfo object with the
// specified message.
func MakeSystemInfo(message string) Composer {
	info, _ := NewSystemInfo(level.Info, message)
	return info
}

// TODO: remove this
func CollectSystemInfoWithLogging() (Composer, []string) {
	return NewSystemInfo(level.Trace, "")
}

// NewSystemInfo returns a fully configured and populated SystemInfo
// object.
func NewSystemInfo(priority level.Priority, message string) (Composer, []string) {
	var err error
	tempLogs := make([]string, 0)
	start := time.Now()
	s := &SystemInfo{
		Message: message,
		NumCPU:  runtime.NumCPU(),
	}

	tempLogs = append(tempLogs, fmt.Sprintf("create sysinfo: %d", time.Since(start)))

	if err = s.SetPriority(priority); err != nil {
		s.Errors = append(s.Errors, err.Error())
		return s, nil
	}

	tempLogs = append(tempLogs, fmt.Sprintf("set priority: %d", time.Since(start)))

	s.loggable = true

	times, err := cpu.Times(false)
	s.saveError("cpu_times", err)
	if err == nil && len(times) > 0 {
		// since we're not storing per-core information,
		// there's only one thing we care about in this struct
		s.CPU = times[0]
	}

	tempLogs = append(tempLogs, fmt.Sprintf("cup_times: %d", time.Since(start)))

	vmstat, err := mem.VirtualMemory()
	s.saveError("vmstat", err)
	if err == nil && vmstat != nil {
		s.VMStat = *vmstat
	}

	tempLogs = append(tempLogs, fmt.Sprintf("virtual_memory: %d", time.Since(start)))

	netstat, err := net.IOCounters(false)
	s.saveError("netstat", err)
	if err == nil && len(netstat) > 0 {
		s.NetStat = netstat[0]
	}

	tempLogs = append(tempLogs, fmt.Sprintf("netstat: %d", time.Since(start)))

	partitions, err := disk.Partitions(true)
	s.saveError("disk_part", err)

	tempLogs = append(tempLogs, fmt.Sprintf("disk_part: %d", time.Since(start)))

	if err == nil {
		var u *disk.UsageStat
		for _, p := range partitions {
			u, err = disk.Usage(p.Mountpoint)
			s.saveError("partition", err)
			if err != nil {
				continue
			}

			s.Usage = append(s.Usage, *u)
		}

		s.Partitions = partitions
	}

	tempLogs = append(tempLogs, fmt.Sprintf("partition: %d", time.Since(start)))

	iostatMap, err := disk.IOCounters()
	s.saveError("iostat", err)
	for _, stat := range iostatMap {
		s.IOStat = append(s.IOStat, stat)
	}

	tempLogs = append(tempLogs, fmt.Sprintf("iostat: %d", time.Since(start)))

	return s, tempLogs
}

// Loggable returns true when the Processinfo structure has been
// populated.
func (s *SystemInfo) Loggable() bool { return s.loggable }

// Raw always returns the SystemInfo object, however it will call the
// Collect method of the base operation first.
func (s *SystemInfo) Raw() interface{} { _ = s.Collect(); return s }

// String returns a string representation of the message, lazily
// rendering the message, and caching it privately.
func (s *SystemInfo) String() string {
	if s.rendered == "" {
		s.rendered = renderStatsString(s.Message, s)
	}

	return s.rendered
}

func (s *SystemInfo) saveError(stat string, err error) {
	if shouldSaveError(err) {
		s.Errors = append(s.Errors, fmt.Sprintf("%s: %v", stat, err))
	}
}

// helper function
func shouldSaveError(err error) bool {
	return err != nil && err.Error() != "not implemented yet"
}

func renderStatsString(msg string, data interface{}) string {
	out, err := json.Marshal(data)
	if err != nil {
		return msg
	}

	if msg == "" {
		return string(out)
	}

	return fmt.Sprintf("%s:\n%s", msg, string(out))
}
