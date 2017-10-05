package message

import (
	"fmt"
	"os"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/net"
	"github.com/shirou/gopsutil/process"
)

// ProcessInfo holds the data for per-process statistics (e.g. cpu,
// memory, io). The Process info composers produce messages in this
// form.
type ProcessInfo struct {
	Message        string                   `json:"message,omitempty" bson:"message,omitempty"`
	Pid            int32                    `json:"pid" bson:"pid"`
	Parent         int32                    `json:"parentPid,omitempty" bson:"parentPid,omitempty"`
	Threads        int                      `json:"numThreads,omitempty" bson:"numThreads,omitempty"`
	Command        string                   `json:"command,omitempty" bson:"command,omitempty"`
	CPU            cpu.TimesStat            `json:"cpu,omitempty" bson:"cpu,omitempty"`
	IoStat         process.IOCountersStat   `json:"io,omitempty" bson:"io,omitempty"`
	NetStat        []net.IOCountersStat     `json:"net,omitempty" bson:"net,omitempty"`
	Memory         process.MemoryInfoStat   `json:"mem,omitempty" bson:"mem,omitempty"`
	MemoryPlatform process.MemoryInfoExStat `json:"memExtra,omitempty" bson:"memExtra,omitempty"`
	Errors         []string                 `json:"errors,omitempty" bson:"errors,omitempty"`
	Base           `json:"metadata,omitempty" bson:"metadata,omitempty"`
	loggable       bool
	rendered       string
}

///////////////////////////////////////////////////////////////////////////
//
// Constructors
//
///////////////////////////////////////////////////////////////////////////

// CollectProcessInfo returns a populated ProcessInfo message.Composer
// instance for the specified pid.
func CollectProcessInfo(pid int32) Composer {
	return NewProcessInfo(level.Trace, pid, "")
}

// CollectProcessInfoSelf returns a populated ProcessInfo message.Composer
// for the pid of the current process.
func CollectProcessInfoSelf() Composer {
	return NewProcessInfo(level.Trace, int32(os.Getpid()), "")
}

// CollectProcessInfoSelfWithChildren returns a slice of populated
// ProcessInfo message.Composer instances for the current process and
// all children processes.
func CollectProcessInfoSelfWithChildren() []Composer {
	p, _ := CollectProcessInfoWithChildren(int32(os.Getpid()))
	return p
}

//TODO: remove
func CollectProcessInfoSelfWithLogging() ([]Composer, []string) {
	return CollectProcessInfoWithChildren(int32(os.Getpid()))
}

// CollectProcessInfoWithChildren returns a slice of populated
// ProcessInfo message.Composer instances for the process with the
// specified pid and all children processes for that process.
func CollectProcessInfoWithChildren(pid int32) ([]Composer, []string) {
	var results []Composer
	tempLogs := make([]string, 0)
	start := time.Now()
	tempLogs = append(tempLogs, fmt.Sprintf("start CollectProcessInfoWithChildren: %d", time.Since(start)))
	parent, err := process.NewProcess(pid)
	if err != nil {
		return results, nil
	}
	tempLogs = append(tempLogs, fmt.Sprintf("create parent process: %d", time.Since(start)))

	parentMsg := &ProcessInfo{}
	parentMsg.loggable = true
	parentMsg.populate(parent)
	results = append(results, parentMsg)
	tempLogs = append(tempLogs, fmt.Sprintf("populate parent: %d", time.Since(start)))

	children := getChildrenRecursively(parent)
	tempLogs = append(tempLogs, fmt.Sprintf("get children: %d", time.Since(start)))
	for _, child := range children {
		cm := &ProcessInfo{}
		cm.loggable = true
		cm.populate(child)
		results = append(results, cm)
		tempLogs = append(tempLogs, fmt.Sprintf("append child %d: %d", child.Pid, time.Since(start)))
	}

	return results, tempLogs
}

func getChildrenRecursively(proc *process.Process) []*process.Process {
	var out []*process.Process

	children, err := proc.Children()
	if len(children) == 0 || err != nil {
		return out
	}

	for _, p := range children {
		out = append(out, p)
		out = append(out, getChildrenRecursively(p)...)
	}

	return out
}

// NewProcessInfo constructs a fully configured and populated
// Processinfo message.Composer instance for the specified process.
func NewProcessInfo(priority level.Priority, pid int32, message string) Composer {
	p := &ProcessInfo{
		Message: message,
		Pid:     pid,
	}

	if err := p.SetPriority(priority); err != nil {
		p.saveError("priority", err)
		return p
	}

	proc, err := process.NewProcess(pid)
	p.saveError("process", err)
	if err != nil {
		return p
	}

	p.loggable = true
	p.populate(proc)

	return p
}

///////////////////////////////////////////////////////////////////////////
//
// message.Composer implementation
//
///////////////////////////////////////////////////////////////////////////

// Loggable returns true when the Processinfo structure has been
// populated.
func (p *ProcessInfo) Loggable() bool { return p.loggable }

// Raw always returns the ProcessInfo object, however it will call the
// Collect method of the base operation first.
func (p *ProcessInfo) Raw() interface{} { _ = p.Collect(); return p }

// String returns a string representation of the message, lazily
// rendering the message, and caching it privately.
func (p *ProcessInfo) String() string {
	if p.rendered == "" {
		p.rendered = renderStatsString(p.Message, p)
	}

	return p.rendered
}

///////////////////////////////////////////////////////////////////////////
//
// Internal Methods for collecting data
//
///////////////////////////////////////////////////////////////////////////

func (p *ProcessInfo) populate(proc *process.Process) {
	var err error

	if p.Pid == 0 {
		p.Pid = proc.Pid
	}
	parentPid, err := proc.Ppid()
	p.saveError("parent_pid", err)
	if err == nil {
		p.Parent = parentPid
	}

	memInfo, err := proc.MemoryInfo()
	p.saveError("meminfo", err)
	if err == nil && memInfo != nil {
		p.Memory = *memInfo
	}

	memInfoEx, err := proc.MemoryInfoEx()
	p.saveError("meminfo_extended", err)
	if err == nil && memInfoEx != nil {
		p.MemoryPlatform = *memInfoEx
	}

	threads, err := proc.NumThreads()
	p.Threads = int(threads)
	p.saveError("num_threads", err)

	p.NetStat, err = proc.NetIOCounters(false)
	p.saveError("netstat", err)

	p.Command, err = proc.Cmdline()
	p.saveError("cmd args", err)

	cpuTimes, err := proc.Times()
	p.saveError("cpu_times", err)
	if err == nil && cpuTimes != nil {
		p.CPU = *cpuTimes
	}

	ioStat, err := proc.IOCounters()
	p.saveError("iostat", err)
	if err == nil && ioStat != nil {
		p.IoStat = *ioStat
	}
}

func (p *ProcessInfo) saveError(stat string, err error) {
	if shouldSaveError(err) {
		p.Errors = append(p.Errors, fmt.Sprintf("%s: %v", stat, err))
	}
}
