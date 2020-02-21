package process

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sort"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/internal/common"
	"github.com/shirou/gopsutil/mem"
)

var (
	invoke                 common.Invoker = common.Invoke{}
	ErrorNoChildren                       = errors.New("process does not have children")
	ErrorProcessNotRunning                = errors.New("process does not exist")
)

type Process struct {
	Pid            int32 `json:"pid" bson:"pid,omitempty"`
	name           string
	status         string
	parent         int32
	numCtxSwitches *NumCtxSwitchesStat
	uids           []int32
	gids           []int32
	numThreads     int32
	memInfo        *MemoryInfoStat
	sigInfo        *SignalInfoStat
	createTime     int64

	lastCPUTimes *cpu.TimesStat
	lastCPUTime  time.Time

	tgid int32
}

type OpenFilesStat struct {
	Path string `json:"path" bson:"path,omitempty"`
	Fd   uint64 `json:"fd" bson:"fd,omitempty"`
}

type MemoryInfoStat struct {
	RSS    uint64 `json:"rss" bson:"rss,omitempty"`       // bytes
	VMS    uint64 `json:"vms" bson:"vms,omitempty"`       // bytes
	HWM    uint64 `json:"hwm" bson:"hwm,omitempty"`       // bytes
	Data   uint64 `json:"data" bson:"data,omitempty"`     // bytes
	Stack  uint64 `json:"stack" bson:"stack,omitempty"`   // bytes
	Locked uint64 `json:"locked" bson:"locked,omitempty"` // bytes
	Swap   uint64 `json:"swap" bson:"swap,omitempty"`     // bytes
}

type SignalInfoStat struct {
	PendingProcess uint64 `json:"pending_process" bson:"pending_process,omitempty"`
	PendingThread  uint64 `json:"pending_thread" bson:"pending_thread,omitempty"`
	Blocked        uint64 `json:"blocked" bson:"blocked,omitempty"`
	Ignored        uint64 `json:"ignored" bson:"ignored,omitempty"`
	Caught         uint64 `json:"caught" bson:"caught,omitempty"`
}

type RlimitStat struct {
	Resource int32  `json:"resource" bson:"resource,omitempty"`
	Soft     int32  `json:"soft" bson:"soft,omitempty"` //TODO too small. needs to be uint64
	Hard     int32  `json:"hard" bson:"hard,omitempty"` //TODO too small. needs to be uint64
	Used     uint64 `json:"used" bson:"used,omitempty"`
}

type IOCountersStat struct {
	ReadCount  uint64 `json:"readCount" bson:"readCount,omitempty"`
	WriteCount uint64 `json:"writeCount" bson:"writeCount,omitempty"`
	ReadBytes  uint64 `json:"readBytes" bson:"readBytes,omitempty"`
	WriteBytes uint64 `json:"writeBytes" bson:"writeBytes,omitempty"`
}

type NumCtxSwitchesStat struct {
	Voluntary   int64 `json:"voluntary" bson:"voluntary,omitempty"`
	Involuntary int64 `json:"involuntary" bson:"involuntary,omitempty"`
}

type PageFaultsStat struct {
	MinorFaults      uint64 `json:"minorFaults" bson:"minorFaults,omitempty"`
	MajorFaults      uint64 `json:"majorFaults" bson:"majorFaults,omitempty"`
	ChildMinorFaults uint64 `json:"childMinorFaults" bson:"childMinorFaults,omitempty"`
	ChildMajorFaults uint64 `json:"childMajorFaults" bson:"childMajorFaults,omitempty"`
}

// Resource limit constants are from /usr/include/x86_64-linux-gnu/bits/resource.h
// from libc6-dev package in Ubuntu 16.10
const (
	RLIMIT_CPU        int32 = 0
	RLIMIT_FSIZE      int32 = 1
	RLIMIT_DATA       int32 = 2
	RLIMIT_STACK      int32 = 3
	RLIMIT_CORE       int32 = 4
	RLIMIT_RSS        int32 = 5
	RLIMIT_NPROC      int32 = 6
	RLIMIT_NOFILE     int32 = 7
	RLIMIT_MEMLOCK    int32 = 8
	RLIMIT_AS         int32 = 9
	RLIMIT_LOCKS      int32 = 10
	RLIMIT_SIGPENDING int32 = 11
	RLIMIT_MSGQUEUE   int32 = 12
	RLIMIT_NICE       int32 = 13
	RLIMIT_RTPRIO     int32 = 14
	RLIMIT_RTTIME     int32 = 15
)

func (p Process) String() string {
	s, _ := json.Marshal(p)
	return string(s)
}

func (o OpenFilesStat) String() string {
	s, _ := json.Marshal(o)
	return string(s)
}

func (m MemoryInfoStat) String() string {
	s, _ := json.Marshal(m)
	return string(s)
}

func (r RlimitStat) String() string {
	s, _ := json.Marshal(r)
	return string(s)
}

func (i IOCountersStat) String() string {
	s, _ := json.Marshal(i)
	return string(s)
}

func (p NumCtxSwitchesStat) String() string {
	s, _ := json.Marshal(p)
	return string(s)
}

// Pids returns a slice of process ID list which are running now.
func Pids() ([]int32, error) {
	return PidsWithContext(context.Background())
}

func PidsWithContext(ctx context.Context) ([]int32, error) {
	pids, err := pidsWithContext(ctx)
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	return pids, err
}

// NewProcess creates a new Process instance, it only stores the pid and
// checks that the process exists. Other method on Process can be used
// to get more information about the process. An error will be returned
// if the process does not exist.
func NewProcess(pid int32) (*Process, error) {
	p := &Process{Pid: pid}

	exists, err := PidExists(pid)
	if err != nil {
		return p, err
	}
	if !exists {
		return p, ErrorProcessNotRunning
	}
	go p.CreateTime()
	return p, nil
}

func PidExists(pid int32) (bool, error) {
	return PidExistsWithContext(context.Background(), pid)
}

// Background returns true if the process is in background, false otherwise.
func (p *Process) Background() (bool, error) {
	return p.BackgroundWithContext(context.Background())
}

func (p *Process) BackgroundWithContext(ctx context.Context) (bool, error) {
	fg, err := p.ForegroundWithContext(ctx)
	if err != nil {
		return false, err
	}
	return !fg, err
}

// If interval is 0, return difference from last call(non-blocking).
// If interval > 0, wait interval sec and return diffrence between start and end.
func (p *Process) Percent(interval time.Duration) (float64, error) {
	return p.PercentWithContext(context.Background(), interval)
}

func (p *Process) PercentWithContext(ctx context.Context, interval time.Duration) (float64, error) {
	cpuTimes, err := p.Times()
	if err != nil {
		return 0, err
	}
	now := time.Now()

	if interval > 0 {
		p.lastCPUTimes = cpuTimes
		p.lastCPUTime = now
		time.Sleep(interval)
		cpuTimes, err = p.Times()
		now = time.Now()
		if err != nil {
			return 0, err
		}
	} else {
		if p.lastCPUTimes == nil {
			// invoked first time
			p.lastCPUTimes = cpuTimes
			p.lastCPUTime = now
			return 0, nil
		}
	}

	numcpu := runtime.NumCPU()
	delta := (now.Sub(p.lastCPUTime).Seconds()) * float64(numcpu)
	ret := calculatePercent(p.lastCPUTimes, cpuTimes, delta, numcpu)
	p.lastCPUTimes = cpuTimes
	p.lastCPUTime = now
	return ret, nil
}

// IsRunning returns whether the process is still running or not.
func (p *Process) IsRunning() (bool, error) {
	return p.IsRunningWithContext(context.Background())
}

func (p *Process) IsRunningWithContext(ctx context.Context) (bool, error) {
	createTime, err := p.CreateTimeWithContext(ctx)
	if err != nil {
		return false, err
	}
	p2, err := NewProcess(p.Pid)
	if err == ErrorProcessNotRunning {
		return false, nil
	}
	createTime2, err := p2.CreateTimeWithContext(ctx)
	if err != nil {
		return false, err
	}
	return createTime == createTime2, nil
}

// CreateTime returns created time of the process in milliseconds since the epoch, in UTC.
func (p *Process) CreateTime() (int64, error) {
	return p.CreateTimeWithContext(context.Background())
}

func (p *Process) CreateTimeWithContext(ctx context.Context) (int64, error) {
	if p.createTime != 0 {
		return p.createTime, nil
	}
	createTime, err := p.createTimeWithContext(ctx)
	p.createTime = createTime
	return p.createTime, err
}

func calculatePercent(t1, t2 *cpu.TimesStat, delta float64, numcpu int) float64 {
	if delta == 0 {
		return 0
	}
	delta_proc := t2.Total() - t1.Total()
	overall_percent := ((delta_proc / delta) * 100) * float64(numcpu)
	return overall_percent
}

// MemoryPercent returns how many percent of the total RAM this process uses
func (p *Process) MemoryPercent() (float32, error) {
	return p.MemoryPercentWithContext(context.Background())
}

func (p *Process) MemoryPercentWithContext(ctx context.Context) (float32, error) {
	machineMemory, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}
	total := machineMemory.Total

	processMemory, err := p.MemoryInfo()
	if err != nil {
		return 0, err
	}
	used := processMemory.RSS

	return (100 * float32(used) / float32(total)), nil
}

// CPU_Percent returns how many percent of the CPU time this process uses
func (p *Process) CPUPercent() (float64, error) {
	return p.CPUPercentWithContext(context.Background())
}

func (p *Process) CPUPercentWithContext(ctx context.Context) (float64, error) {
	crt_time, err := p.CreateTime()
	if err != nil {
		return 0, err
	}

	cput, err := p.Times()
	if err != nil {
		return 0, err
	}

	created := time.Unix(0, crt_time*int64(time.Millisecond))
	totalTime := time.Since(created).Seconds()
	if totalTime <= 0 {
		return 0, nil
	}

	return 100 * cput.Total() / totalTime, nil
}
