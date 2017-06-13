package model

import (
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type APISystemMetrics struct {
	CPU        APICPUMetrics                `json:"cpu"`
	NumCPU     int                          `json:"num_cpus"`
	NetStat    APINetStatMetrics            `json:"netstat"`
	Partitions []APISystemMetricsPartitions `json:"partitions"`
	Usage      []APISystemMetricsDiskUsage  `json:"usage"`
	IOStat     []APISystemMetricsIOStat     `json:"iostat"`
	VMStat     struct {
		Total        uint64  `json:"total"`
		Available    uint64  `json:"available"`
		Used         uint64  `json:"used"`
		UsedPercent  float64 `json:"usedPercent"`
		Free         uint64  `json:"free"`
		Active       uint64  `json:"active"`
		Inactive     uint64  `json:"inactive"`
		Wired        uint64  `json:"wired"`
		Buffers      uint64  `json:"buffers"`
		Cached       uint64  `json:"cached"`
		Writeback    uint64  `json:"writeback"`
		Dirty        uint64  `json:"dirty"`
		WritebackTmp uint64  `json:"writebacktmp"`
		Shared       uint64  `json:"shared"`
		Slab         uint64  `json:"slab"`
		PageTables   uint64  `json:"pagetables"`
		SwapCached   uint64  `json:"swapcached"`
	} `json:"vmstat"`
}

type APICPUMetrics struct {
	CPU       APIString `json:"cpu"`
	User      float64   `json:"user"`
	System    float64   `json:"system"`
	Idle      float64   `json:"idle"`
	Nice      float64   `json:"nice"`
	Iowait    float64   `json:"iowait"`
	Irq       float64   `json:"irq"`
	Softirq   float64   `json:"softirq"`
	Steal     float64   `json:"steal"`
	Guest     float64   `json:"guest"`
	GuestNice float64   `json:"guestNice"`
	Stolen    float64   `json:"stolen"`
}

type APINetStatMetrics struct {
	Name        APIString `json:"name"`
	BytesSent   uint64    `json:"bytesSent"`
	BytesRecv   uint64    `json:"bytesRecv"`
	PacketsSent uint64    `json:"packetsSent"`
	PacketsRecv uint64    `json:"packetsRecv"`
	Errin       uint64    `json:"errin"`
	Errout      uint64    `json:"errout"`
	Dropin      uint64    `json:"dropin"`
	Dropout     uint64    `json:"dropout"`
	Fifoin      uint64    `json:"fifoin"`
	Fifoout     uint64    `json:"fifoout"`
}

type APISystemMetricsPartitions struct {
	Device     APIString `json:"device"`
	Mountpoint APIString `json:"mountpoint"`
	Fstype     APIString `json:"fstype"`
	Opts       APIString `json:"opts"`
}

type APISystemMetricsDiskUsage struct {
	Path              APIString `json:"path"`
	Fstype            APIString `json:"fstype"`
	Total             uint64    `json:"total"`
	Free              uint64    `json:"free"`
	Used              uint64    `json:"used"`
	UsedPercent       float64   `json:"usedPercent"`
	InodesTotal       uint64    `json:"inodesTotal"`
	InodesUsed        uint64    `json:"inodesUsed"`
	InodesFree        uint64    `json:"inodesFree"`
	InodesUsedPercent float64   `json:"inodesUsedPercent"`
}

type APISystemMetricsIOStat struct {
	Name             APIString `json:"name"`
	ReadCount        uint64    `json:"readCount"`
	MergedReadCount  uint64    `json:"mergedReadCount"`
	WriteCount       uint64    `json:"writeCount"`
	MergedWriteCount uint64    `json:"mergedWriteCount"`
	ReadBytes        uint64    `json:"readBytes"`
	WriteBytes       uint64    `json:"writeBytes"`
	ReadTime         uint64    `json:"readTime"`
	WriteTime        uint64    `json:"writeTime"`
	IopsInProgress   uint64    `json:"iopsInProgress"`
	IoTime           uint64    `json:"ioTime"`
	WeightedIO       uint64    `json:"weightedIO"`
}

func (m *APISystemMetrics) BuildFromService(in interface{}) error {
	source, ok := in.(*message.SystemInfo)
	if !ok {
		return errors.Errorf("could not construct api model from %T system metrics", in)
	}

	m.NumCPU = source.NumCPU
	m.CPU.CPU = APIString(source.CPU.CPU)
	m.CPU.User = source.CPU.User
	m.CPU.System = source.CPU.System
	m.CPU.Idle = source.CPU.Idle
	m.CPU.Nice = source.CPU.Nice
	m.CPU.Iowait = source.CPU.Iowait
	m.CPU.Irq = source.CPU.Irq
	m.CPU.Softirq = source.CPU.Softirq
	m.CPU.Steal = source.CPU.Steal
	m.CPU.Guest = source.CPU.Guest
	m.CPU.GuestNice = source.CPU.GuestNice
	m.CPU.Stolen = source.CPU.Stolen

	m.VMStat.Total = source.VMStat.Total
	m.VMStat.Available = source.VMStat.Available
	m.VMStat.Used = source.VMStat.Used
	m.VMStat.UsedPercent = source.VMStat.UsedPercent
	m.VMStat.Free = source.VMStat.Free
	m.VMStat.Active = source.VMStat.Active
	m.VMStat.Inactive = source.VMStat.Inactive
	m.VMStat.Wired = source.VMStat.Wired
	m.VMStat.Buffers = source.VMStat.Buffers
	m.VMStat.Cached = source.VMStat.Cached
	m.VMStat.Writeback = source.VMStat.Writeback
	m.VMStat.Dirty = source.VMStat.Dirty
	m.VMStat.WritebackTmp = source.VMStat.WritebackTmp
	m.VMStat.Shared = source.VMStat.Shared
	m.VMStat.Slab = source.VMStat.Slab
	m.VMStat.PageTables = source.VMStat.PageTables
	m.VMStat.SwapCached = source.VMStat.SwapCached

	m.NetStat.Name = APIString(source.NetStat.Name)
	m.NetStat.BytesSent = source.NetStat.BytesSent
	m.NetStat.BytesRecv = source.NetStat.BytesRecv
	m.NetStat.PacketsSent = source.NetStat.PacketsSent
	m.NetStat.PacketsRecv = source.NetStat.PacketsRecv
	m.NetStat.Errin = source.NetStat.Errin
	m.NetStat.Errout = source.NetStat.Errout
	m.NetStat.Dropin = source.NetStat.Dropin
	m.NetStat.Dropout = source.NetStat.Dropout
	m.NetStat.Fifoin = source.NetStat.Fifoin
	m.NetStat.Fifoout = source.NetStat.Fifoout

	for _, part := range source.Partitions {
		m.Partitions = append(m.Partitions, APISystemMetricsPartitions{
			Device:     APIString(part.Device),
			Mountpoint: APIString(part.Mountpoint),
			Fstype:     APIString(part.Fstype),
			Opts:       APIString(part.Opts),
		})
	}

	for _, usage := range source.Usage {
		m.Usage = append(m.Usage, APISystemMetricsDiskUsage{
			Path:              APIString(usage.Path),
			Fstype:            APIString(usage.Fstype),
			Total:             usage.Total,
			Free:              usage.Free,
			Used:              usage.Used,
			UsedPercent:       usage.UsedPercent,
			InodesTotal:       usage.InodesTotal,
			InodesUsed:        usage.InodesUsed,
			InodesFree:        usage.InodesFree,
			InodesUsedPercent: usage.InodesUsedPercent,
		})

	}

	for _, stat := range source.IOStat {
		m.IOStat = append(m.IOStat, APISystemMetricsIOStat{
			Name:             APIString(stat.Name),
			ReadCount:        stat.ReadCount,
			MergedReadCount:  stat.MergedReadCount,
			WriteCount:       stat.WriteCount,
			MergedWriteCount: stat.MergedWriteCount,
			ReadBytes:        stat.ReadBytes,
			WriteBytes:       stat.WriteBytes,
			ReadTime:         stat.ReadTime,
			WriteTime:        stat.WriteTime,
			IopsInProgress:   stat.IopsInProgress,
			IoTime:           stat.IoTime,
			WeightedIO:       stat.WeightedIO,
		})
	}

	return nil
}

func (m *APISystemMetrics) ToService() (interface{}, error) {
	return nil, errors.New("the api for metrics data is read only")
}

type APIProcessMetrics []APIProcessStat

type APIProcessStat struct {
	Pid     int32         `json:"pid"`
	Parent  int32         `json:"parent"`
	Threads int           `json:"num_threads"`
	Command APIString     `json:"command"`
	CPU     APICPUMetrics `json:"cpu"`
	IOStat  struct {
		ReadCount  uint64 `json:"readCount"`
		WriteCount uint64 `json:"writeCount"`
		ReadBytes  uint64 `json:"readBytes"`
		WriteBytes uint64 `json:"writeBytes"`
	} `json:"iostat"`
	NetStat []APINetStatMetrics `json:"netstat"`
	Memory  struct {
		RSS  uint64 `json:"rss"`
		VMS  uint64 `json:"vms"`
		Swap uint64 `json:"swap"`
	} `json:"memory"`
	MemoryExtended interface{} `json:"memory_extended"`
}

func (m *APIProcessMetrics) BuildFromService(in interface{}) error {
	data := []*message.ProcessInfo{}

	// convert types as needed; try and avoid heroic conversions in calling code.
	switch in := in.(type) {
	case []*message.ProcessInfo:
		data = in
	case *message.ProcessInfo:
		data = append(data, in)
	case []message.Composer:
		for _, p := range in {
			proc, ok := p.(*message.ProcessInfo)
			if !ok {
				return errors.Errorf("cannot support convefrting %T to process metrics data", proc)
			}

			data = append(data, proc)
		}
	default:
		return errors.Errorf("cannot support convefrting %T to process metrics data", in)
	}

	// begin building output

	out := []APIProcessStat{}
	for _, proc := range data {
		stat := APIProcessStat{
			Pid:     proc.Pid,
			Parent:  proc.Parent,
			Threads: proc.Threads,
			Command: APIString(proc.Command),
			CPU: APICPUMetrics{
				CPU:       APIString(proc.CPU.CPU),
				User:      proc.CPU.User,
				System:    proc.CPU.System,
				Idle:      proc.CPU.Idle,
				Nice:      proc.CPU.Nice,
				Iowait:    proc.CPU.Iowait,
				Irq:       proc.CPU.Irq,
				Softirq:   proc.CPU.Softirq,
				Steal:     proc.CPU.Steal,
				Guest:     proc.CPU.Guest,
				GuestNice: proc.CPU.GuestNice,
				Stolen:    proc.CPU.Stolen,
			},
			MemoryExtended: proc.MemoryPlatform,
		}

		stat.IOStat.ReadBytes = proc.IoStat.ReadBytes
		stat.IOStat.ReadCount = proc.IoStat.ReadCount
		stat.IOStat.WriteBytes = proc.IoStat.WriteBytes
		stat.IOStat.WriteCount = proc.IoStat.WriteCount

		for _, net := range proc.NetStat {
			stat.NetStat = append(stat.NetStat, APINetStatMetrics{
				Name:        APIString(net.Name),
				BytesSent:   net.BytesSent,
				BytesRecv:   net.BytesRecv,
				PacketsSent: net.PacketsSent,
				PacketsRecv: net.PacketsRecv,
				Errin:       net.Errin,
				Errout:      net.Errout,
				Dropin:      net.Dropin,
				Dropout:     net.Dropout,
				Fifoin:      net.Fifoin,
				Fifoout:     net.Fifoout,
			})
		}

		stat.Memory.RSS = proc.Memory.RSS
		stat.Memory.VMS = proc.Memory.VMS
		stat.Memory.Swap = proc.Memory.Swap

		out = append(out, stat)
	}

	*m = out

	return nil
}

func (m *APIProcessMetrics) ToService() (interface{}, error) {
	return nil, errors.New("the api for metrics data is read only")
}
