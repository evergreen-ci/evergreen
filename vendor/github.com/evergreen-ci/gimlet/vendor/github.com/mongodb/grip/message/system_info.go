package message

import (
	"encoding/json"
	"fmt"
	"runtime"

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
	Message    string                `json:"message" bson:"message"`
	CPU        StatCPUTimes          `json:"cpu" bson:"cpu"`
	CPUPercent float64               `json:"cpu_percent" bson:"cpu_percent"`
	NumCPU     int                   `json:"num_cpus" bson:"num_cpus"`
	VMStat     mem.VirtualMemoryStat `json:"vmstat" bson:"vmstat"`
	NetStat    net.IOCountersStat    `json:"netstat" bson:"netstat"`
	Partitions []disk.PartitionStat  `json:"partitions" bson:"partitions"`
	Usage      []disk.UsageStat      `json:"usage" bson:"usage"`
	IOStat     []disk.IOCountersStat `json:"iostat" bson:"iostat"`
	Errors     []string              `json:"errors" bson:"errors"`
	Base       `json:"metadata,omitempty" bson:"metadata,omitempty"`
	loggable   bool
	rendered   string
}

// StatCPUTimes provides a mirror of gopsutil/cpu.TimesStat with
// integers rather than floats.
type StatCPUTimes struct {
	User      int64 `json:"user" bson:"user"`
	System    int64 `json:"system" bson:"system"`
	Idle      int64 `json:"idle" bson:"idle"`
	Nice      int64 `json:"nice" bson:"nice"`
	Iowait    int64 `json:"iowait" bson:"iowait"`
	Irq       int64 `json:"irq" bson:"irq"`
	Softirq   int64 `json:"softirq" bson:"softirq"`
	Steal     int64 `json:"steal" bson:"steal"`
	Guest     int64 `json:"guest" bson:"guest"`
	GuestNice int64 `json:"guestNice" bson:"guestNice"`
}

func convertCPUTimes(in cpu.TimesStat) StatCPUTimes {
	return StatCPUTimes{
		User:      int64(in.User * cpuTicks),
		System:    int64(in.System * cpuTicks),
		Idle:      int64(in.Idle * cpuTicks),
		Nice:      int64(in.Nice * cpuTicks),
		Iowait:    int64(in.Iowait * cpuTicks),
		Irq:       int64(in.Irq * cpuTicks),
		Softirq:   int64(in.Softirq * cpuTicks),
		Steal:     int64(in.Steal * cpuTicks),
		Guest:     int64(in.Guest * cpuTicks),
		GuestNice: int64(in.GuestNice * cpuTicks),
	}
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
	var err error
	s := &SystemInfo{
		Message: message,
		NumCPU:  runtime.NumCPU(),
	}

	if err = s.SetPriority(priority); err != nil {
		s.Errors = append(s.Errors, err.Error())
		return s
	}

	s.loggable = true

	times, err := cpu.Times(false)
	s.saveError("cpu_times", err)
	if err == nil && len(times) > 0 {
		// since we're not storing per-core information,
		// there's only one thing we care about in this struct
		s.CPU = convertCPUTimes(times[0])
	}
	percent, err := cpu.Percent(0, false)
	if err != nil {
		s.saveError("cpu_times", err)
	} else {
		s.CPUPercent = percent[0]
	}

	vmstat, err := mem.VirtualMemory()
	s.saveError("vmstat", err)
	if err == nil && vmstat != nil {
		s.VMStat = *vmstat
		s.VMStat.UsedPercent = 0.0
	}

	netstat, err := net.IOCounters(false)
	s.saveError("netstat", err)
	if err == nil && len(netstat) > 0 {
		s.NetStat = netstat[0]
	}

	partitions, err := disk.Partitions(true)
	s.saveError("disk_part", err)

	if err == nil {
		var u *disk.UsageStat
		for _, p := range partitions {
			u, err = disk.Usage(p.Mountpoint)
			s.saveError("partition", err)
			if err != nil {
				continue
			}
			u.UsedPercent = 0.0
			u.InodesUsedPercent = 0.0

			s.Usage = append(s.Usage, *u)
		}

		s.Partitions = partitions
	}

	iostatMap, err := disk.IOCounters()
	s.saveError("iostat", err)
	for _, stat := range iostatMap {
		s.IOStat = append(s.IOStat, stat)
	}

	return s
}

// Loggable returns true when the Processinfo structure has been
// populated.
func (s *SystemInfo) Loggable() bool { return s.loggable }

// Raw always returns the SystemInfo object.
func (s *SystemInfo) Raw() interface{} { return s }

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
