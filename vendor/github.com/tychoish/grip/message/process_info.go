package message

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/net"
	"github.com/shirou/gopsutil/process"
	"github.com/tychoish/grip/level"
)

// ProcessInfo holds the data for per-process statistics (e.g. cpu,
// memory, io). The Process info composers produce messages in this
// form.
type ProcessInfo struct {
	Message        string                        `json:"message,omitempty"`
	Pid            int32                         `json:"pid"`
	Parent         int32                         `json:"parentPid,omitempty"`
	Threads        int                           `json:"numThreads,omitempty"`
	Command        string                        `json:"command,omitempty"`
	IoStat         *process.IOCountersStat       `json:"io,omitempty"`
	MemoryPlatform *process.MemoryInfoExStat     `json:"memExtra,omitempty"`
	Memory         *process.MemoryInfoStat       `json:"mem,omitempty"`
	Network        map[string]net.IOCountersStat `json:"net,omitempty"`
	CPU            *cpu.TimesStat                `json:"cpu,omitempty"`
	Errors         []string                      `json:"errors,omitempty"`
	Base           `json:"metadata"`
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
	return CollectProcessInfoWithChildren(int32(os.Getpid()))
}

// CollectProcessInfoWithChildren returns a slice of populated
// ProcessInfo message.Composer instances for the process with the
// specified pid and all children processes for that process.
func CollectProcessInfoWithChildren(pid int32) []Composer {
	var results []Composer
	parent, err := process.NewProcess(pid)
	if err != nil {
		return results
	}

	parentMsg := &ProcessInfo{}
	parentMsg.loggable = true
	parentMsg.populate(parent)
	results = append(results, parentMsg)

	children, err := parent.Children()
	parentMsg.saveError(err)
	if err != nil {
		return results
	}

	for _, child := range children {
		cm := &ProcessInfo{}
		cm.loggable = true
		cm.populate(child)
		results = append(results, cm)
	}

	return results
}

// NewProcessInfo constructs a fully configured and populated
// Processinfo message.Composer instance for the specified process.
func NewProcessInfo(priority level.Priority, pid int32, message string) Composer {
	p := &ProcessInfo{
		Message: message,
		Pid:     pid,
	}

	if err := p.SetPriority(priority); err != nil {
		p.saveError(err)
		return p
	}

	proc, err := process.NewProcess(pid)
	p.saveError(err)
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

// Resolve returns a string representation of the message, lazily
// rendering the message, and caching it privately.
func (p *ProcessInfo) Resolve() string {
	if p.rendered != "" {
		return p.rendered
	}

	data, err := json.MarshalIndent(p, "  ", " ")
	if err != nil {
		return p.Message
	}

	if p.Message == "" {
		p.rendered = string(data)
	} else {
		p.rendered = fmt.Sprintf("%s:\n%s", p.Message, string(data))
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

	p.Pid = proc.Pid

	p.Parent, err = proc.Ppid()
	p.saveError(err)

	p.Memory, err = proc.MemoryInfo()
	p.saveError(err)

	p.MemoryPlatform, err = proc.MemoryInfoEx()
	p.saveError(err)

	threads, err := proc.NumThreads()
	p.Threads = int(threads)
	p.saveError(err)

	netStat, err := proc.NetIOCounters(false)
	p.saveError(err)
	if err == nil {
		p.Network = make(map[string]net.IOCountersStat)
		for _, stat := range netStat {
			p.Network[stat.Name] = stat
		}
	}

	p.Command, err = proc.Cmdline()
	p.saveError(err)

	p.CPU, err = proc.Times()
	p.saveError(err)

	p.IoStat, err = proc.IOCounters()
	p.saveError(err)
}

func (p *ProcessInfo) saveError(err error) {
	if shouldSaveError(err) {
		p.Errors = append(p.Errors, err.Error())
	}
}
