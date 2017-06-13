package event

import (
	"time"

	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const (
	EventTaskSystemInfo  = "TASK_SYSTEM_INFO"
	EventTaskProcessInfo = "TASK_PROCESS_INFO"
)

// TaskSystemResourceData wraps a grip/message.SystemInfo struct in a
// type that implements the event.Data interface. SystemInfo structs
// capture aggregated system metrics (cpu, memory, network) for the
// system as a whole.
type TaskSystemResourceData struct {
	ResourceType string              `bson:"r_type" json:"resource_type"`
	SystemInfo   *message.SystemInfo `bson:"system_info" json:"system_info" yaml:"system_info"`
}

var (
	TaskSystemResourceDataSysInfoKey      = bsonutil.MustHaveTag(TaskSystemResourceData{}, "SystemInfo")
	TaskSystemResourceDataResourceTypeKey = bsonutil.MustHaveTag(TaskSystemResourceData{}, "ResourceType")

	SysInfoCPUKey      = bsonutil.MustHaveTag(message.SystemInfo{}, "CPU")
	SysInfoNumCPUKey   = bsonutil.MustHaveTag(message.SystemInfo{}, "NumCPU")
	SysInfoVMStatKey   = bsonutil.MustHaveTag(message.SystemInfo{}, "VMStat")
	SysInfoNetStatKey  = bsonutil.MustHaveTag(message.SystemInfo{}, "NetStat")
	SysInfoPartionsKey = bsonutil.MustHaveTag(message.SystemInfo{}, "Partitions")
	SysInfoUsageKey    = bsonutil.MustHaveTag(message.SystemInfo{}, "Usage")
	SysInfoIoStatKey   = bsonutil.MustHaveTag(message.SystemInfo{}, "IOStat")
	SysInfoErrorsKey   = bsonutil.MustHaveTag(message.SystemInfo{}, "Errors")

	SysInfoCPUTimesStatCPUKey         = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "CPU")
	SysInfoCPUTimesStatUserKey        = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "User")
	SysInfoCPUTimesStatSystemKey      = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "System")
	SysInfoCPUTimesStatIdleKey        = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "Idle")
	SysInfoCPUTimesStatNiceKey        = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "Nice")
	SysInfoCPUTimesStatIowaitKey      = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "Iowait")
	SysInfoCPUTimesStatIrqKey         = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "Irq")
	SysInfoCPUTimesStatSoftirqKey     = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "Softirq")
	SysInfoCPUTimesStatStealKey       = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "Steal")
	SysInfoCPUTimesStatGuestNiceKey   = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "GuestNice")
	SysInfoCPUTimesStatGuestStolenKey = bsonutil.MustHaveTag(message.SystemInfo{}.CPU, "Stolen")

	SysInfoVMStatTotalKey        = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Total")
	SysInfoVMStatAvailableKey    = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Available")
	SysInfoVMStatUsedKey         = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Used")
	SysInfoVMStatUsedPercentKey  = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "UsedPercent")
	SysInfoVMStatFreeKey         = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Free")
	SysInfoVMStatActiveKey       = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Active")
	SysInfoVMStatInactiveKey     = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Inactive")
	SysInfoVMStatWiredKey        = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Wired")
	SysInfoVMStatBuffersKey      = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Buffers")
	SysInfoVMStatCachedKey       = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Cached")
	SysInfoVMStatWritebackKey    = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Writeback")
	SysInfoVMStatDirtyKey        = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Dirty")
	SysInfoVMStatWritebackTmpKey = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "WritebackTmp")
	SysInfoVMStatSharedKey       = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Shared")
	SysInfoVMStatSlabKey         = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "Slab")
	SysInfoVMStatPageTablesKey   = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "PageTables")
	SysInfoVMStatSwapCachedKey   = bsonutil.MustHaveTag(message.SystemInfo{}.VMStat, "SwapCached")

	SysInfoNetIOCountersNameKey        = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "Name")
	SysInfoNetIOCountersBytesSentKey   = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "BytesSent")
	SysInfoNetIOCountersBytesRecvKey   = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "BytesRecv")
	SysInfoNetIOCountersPacketsSentKey = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "PacketsSent")
	SysInfoNetIOCountersPacketsRecvKey = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "PacketsRecv")
	SysInfoNetIOCountersErrinKey       = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "Errin")
	SysInfoNetIOCountersErroutKey      = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "Errout")
	SysInfoNetIOCountersDropinKey      = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "Dropin")
	SysInfoNetIOCountersDropoutKey     = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "Dropout")
	SysInfoNetIOCountersFifoinKey      = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "Fifoin")
	SysInfoNetIOCountersFifooutKey     = bsonutil.MustHaveTag(message.SystemInfo{}.NetStat, "Fifoout")

	SysInfoPartitionStatDeviceKey     = bsonutil.MustHaveTag(message.SystemInfo{}.Partitions, "Device")
	SysInfoPartitionStatMountpointKey = bsonutil.MustHaveTag(message.SystemInfo{}.Partitions, "Mountpoint")
	SysInfoPartitionStatFSTypeKey     = bsonutil.MustHaveTag(message.SystemInfo{}.Partitions, "Fstype")
	SysInfoPartitionStatOptsKey       = bsonutil.MustHaveTag(message.SystemInfo{}.Partitions, "Opts")

	SysInfoUsageStatPathKey              = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "Path")
	SysInfoUsageStatFSTypeKey            = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "Fstype")
	SysInfoUsageStatTotalKey             = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "Total")
	SysInfoUsageStatFreeKey              = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "Free")
	SysInfoUsageStatUsedKey              = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "Used")
	SysInfoUsageStatUsedPercentKey       = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "UsedPercent")
	SysInfoUsageStatInodesTotalKey       = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "InodesTotal")
	SysInfoUsageStatInodesUsedKey        = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "InodesUsed")
	SysInfoUsageStatInodesFreeKey        = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "InodesFree")
	SysInfoUsageStatInodesUsedPercentKey = bsonutil.MustHaveTag(message.SystemInfo{}.Usage, "InodesUsedPercent")

	SysInfoDiskIOCountersReadCountKey        = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "ReadCount")
	SysInfoDiskIOCountersMergedReadCountKey  = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "MergedReadCount")
	SysInfoDiskIOCountersWriteCountKey       = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "WriteCount")
	SysInfoDiskIOCountersMergedWriteCountKey = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "MergedWriteCount")
	SysInfoDiskIOCountersReadBytesKey        = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "ReadBytes")
	SysInfoDiskIOCountersWriteBytesKey       = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "WriteBytes")
	SysInfoDiskIOCountersReadTimeKey         = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "ReadTime")
	SysInfoDiskIOCountersWriteTimeKey        = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "WriteTime")
	SysInfoDiskIOCountersIopsInProgressKey   = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "IopsInProgress")
	SysInfoDiskIOCountersIoTimeKey           = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "IoTime")
	SysInfoDiskIOCountersWeigtedIoKey        = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "WeightedIO")
	SysInfoDiskIOCountersNameKey             = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "Name")
	SysInfoDiskIOCountersSerialNumberKey     = bsonutil.MustHaveTag(message.SystemInfo{}.IOStat, "SerialNumber")
)

// IsValid is part of the Data interface used in the conversion of
// Event documents from to TaskSystemResourceData.
func (d TaskSystemResourceData) IsValid() bool {
	return d.ResourceType == EventTaskSystemInfo
}

// LogTaskSystemData saves a SystemInfo object to the event log for a
// task.
func LogTaskSystemData(taskId string, info *message.SystemInfo) {
	event := Event{
		ResourceId: taskId,
		Timestamp:  info.Base.Time,
		EventType:  EventTaskSystemInfo,
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
		info.Base = message.Base{Time: event.Timestamp}
	}

	data := TaskSystemResourceData{
		ResourceType: EventTaskSystemInfo,
		SystemInfo:   info,
	}
	event.Data = DataWrapper{data}

	grip.Error(message.NewErrorWrap(NewDBEventLogger(TaskLogCollection).LogEvent(event),
		"problem system info event"))
}

// TaskProcessResourceData wraps a slice of grip/message.ProcessInfo structs
// in a type that implements the event.Data interface. ProcessInfo structs
// represent system resource usage information for a single process (PID).
type TaskProcessResourceData struct {
	ResourceType string                 `bson:"r_type" json:"resource_type"`
	Processes    []*message.ProcessInfo `bson:"processes" json:"processes"`
}

var (
	TaskProcessResourceDataSysInfoKey      = bsonutil.MustHaveTag(TaskProcessResourceData{}, "Processes")
	TaskProcessResourceDataResourceTypeKey = bsonutil.MustHaveTag(TaskProcessResourceData{}, "ResourceType")

	ProcInfoPidKey     = bsonutil.MustHaveTag(message.ProcessInfo{}, "Pid")
	ProcInfoParentKey  = bsonutil.MustHaveTag(message.ProcessInfo{}, "Parent")
	ProcInfoThreadsKey = bsonutil.MustHaveTag(message.ProcessInfo{}, "Threads")
	ProcInfoCommandKey = bsonutil.MustHaveTag(message.ProcessInfo{}, "Command")
	ProcInfoErrorsKey  = bsonutil.MustHaveTag(message.ProcessInfo{}, "Errors")

	ProcInfoCPUKey            = bsonutil.MustHaveTag(message.ProcessInfo{}, "CPU")
	ProcInfoIoStatKey         = bsonutil.MustHaveTag(message.ProcessInfo{}, "IoStat")
	ProcInfoMemoryKey         = bsonutil.MustHaveTag(message.ProcessInfo{}, "Memory")
	ProcInfoMemoryPlatformKey = bsonutil.MustHaveTag(message.ProcessInfo{}, "MemoryPlatform")
	ProcInfoNetStatKey        = bsonutil.MustHaveTag(message.ProcessInfo{}, "NetStat")

	ProcInfoCPUTimesStatCPUKey         = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "CPU")
	ProcInfoCPUTimesStatUserKey        = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "User")
	ProcInfoCPUTimesStatSystemKey      = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "System")
	ProcInfoCPUTimesStatIdleKey        = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "Idle")
	ProcInfoCPUTimesStatNiceKey        = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "Nice")
	ProcInfoCPUTimesStatIowaitKey      = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "Iowait")
	ProcInfoCPUTimesStatIrqKey         = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "Irq")
	ProcInfoCPUTimesStatSoftirqKey     = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "Softirq")
	ProcInfoCPUTimesStatStealKey       = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "Steal")
	ProcInfoCPUTimesStatGuestNiceKey   = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "GuestNice")
	ProcInfoCPUTimesStatGuestStolenKey = bsonutil.MustHaveTag(message.ProcessInfo{}.CPU, "Stolen")

	ProcInfoIoStatReadCountKey  = bsonutil.MustHaveTag(message.ProcessInfo{}.IoStat, "ReadCount")
	ProcInfoIoStatReadBytesKey  = bsonutil.MustHaveTag(message.ProcessInfo{}.IoStat, "ReadBytes")
	ProcInfoIoStatWriteCountKey = bsonutil.MustHaveTag(message.ProcessInfo{}.IoStat, "WriteCount")
	ProcInfoIoStatWriteBytesKey = bsonutil.MustHaveTag(message.ProcessInfo{}.IoStat, "WriteBytes")

	ProcInfoNetStatNameKey        = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "Name")
	ProcInfoNetStatBytesSentKey   = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "BytesSent")
	ProcInfoNetStatBytesRecvKey   = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "BytesRecv")
	ProcInfoNetStatPacketsSentKey = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "PacketsSent")
	ProcInfoNetStatPacketsRecvKey = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "PacketsRecv")
	ProcInfoNetStatErrinKey       = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "Errin")
	ProcInfoNetStatErroutKey      = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "Errout")
	ProcInfoNetStatDropinKey      = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "Dropin")
	ProcInfoNetStatDropoutKey     = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "Dropout")
	ProcInfoNetStatFifoinKey      = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "Fifoin")
	ProcInfoNetStatFifooutKey     = bsonutil.MustHaveTag(message.ProcessInfo{}.NetStat, "Fifoout")

	ProcInfoMemInfoStatRSSKey  = bsonutil.MustHaveTag(message.ProcessInfo{}.Memory, "RSS")
	ProcInfoMemInfoStatVMSKey  = bsonutil.MustHaveTag(message.ProcessInfo{}.Memory, "VMS")
	ProcInfoMemInfoStatSwapKey = bsonutil.MustHaveTag(message.ProcessInfo{}.Memory, "Swap")
)

// IsValid is part of the Data interface used in the conversion of
// Event documents from to TaskProcessResourceData..
func (d TaskProcessResourceData) IsValid() bool {
	return d.ResourceType == EventTaskProcessInfo
}

// LogTaskProcessData saves a slice of ProcessInfo objects to the
// event log under the specified task.
func LogTaskProcessData(taskId string, procs []*message.ProcessInfo) {
	ts := time.Now()
	b := message.Base{}
	for _, p := range procs {
		// if p.Parent is 0, then this is the root of the
		// process, and we should use the timestamp from this
		// collector.
		if p.Parent == 0 && !p.Base.Time.IsZero() {
			ts = p.Base.Time
			b.Time = ts
		}
		p.Base = b
	}

	data := TaskProcessResourceData{
		ResourceType: EventTaskProcessInfo,
		Processes:    procs,
	}

	event := Event{
		Timestamp:  ts,
		ResourceId: taskId,
		EventType:  EventTaskProcessInfo,
		Data:       DataWrapper{data},
	}

	grip.Error(message.NewErrorWrap(NewDBEventLogger(TaskLogCollection).LogEvent(event),
		"problem logging task process info event"))
}
