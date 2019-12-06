package disk

import (
	"encoding/json"

	"github.com/shirou/gopsutil/internal/common"
)

var invoke common.Invoker = common.Invoke{}

type UsageStat struct {
	Path              string  `json:"path" bson:"path,omitempty"`
	Fstype            string  `json:"fstype" bson:"fstype,omitempty"`
	Total             uint64  `json:"total" bson:"total,omitempty"`
	Free              uint64  `json:"free" bson:"free,omitempty"`
	Used              uint64  `json:"used" bson:"used,omitempty"`
	UsedPercent       float64 `json:"usedPercent" bson:"usedPercent,omitempty"`
	InodesTotal       uint64  `json:"inodesTotal" bson:"inodesTotal,omitempty"`
	InodesUsed        uint64  `json:"inodesUsed" bson:"inodesUsed,omitempty"`
	InodesFree        uint64  `json:"inodesFree" bson:"inodesFree,omitempty"`
	InodesUsedPercent float64 `json:"inodesUsedPercent" bson:"inodesUsedPercent,omitempty"`
}

type PartitionStat struct {
	Device     string `json:"device" bson:"device,omitempty"`
	Mountpoint string `json:"mountpoint" bson:"mountpoint,omitempty"`
	Fstype     string `json:"fstype" bson:"fstype,omitempty"`
	Opts       string `json:"opts" bson:"opts,omitempty"`
}

type IOCountersStat struct {
	ReadCount        uint64 `json:"readCount" bson:"readCount,omitempty"`
	MergedReadCount  uint64 `json:"mergedReadCount" bson:"mergedReadCount,omitempty"`
	WriteCount       uint64 `json:"writeCount" bson:"writeCount,omitempty"`
	MergedWriteCount uint64 `json:"mergedWriteCount" bson:"mergedWriteCount,omitempty"`
	ReadBytes        uint64 `json:"readBytes" bson:"readBytes,omitempty"`
	WriteBytes       uint64 `json:"writeBytes" bson:"writeBytes,omitempty"`
	ReadTime         uint64 `json:"readTime" bson:"readTime,omitempty"`
	WriteTime        uint64 `json:"writeTime" bson:"writeTime,omitempty"`
	IopsInProgress   uint64 `json:"iopsInProgress" bson:"iopsInProgress,omitempty"`
	IoTime           uint64 `json:"ioTime" bson:"ioTime,omitempty"`
	WeightedIO       uint64 `json:"weightedIO" bson:"weightedIO,omitempty"`
	Name             string `json:"name" bson:"name,omitempty"`
	SerialNumber     string `json:"serialNumber" bson:"serialNumber,omitempty"`
	Label            string `json:"label" bson:"label,omitempty"`
}

func (d UsageStat) String() string {
	s, _ := json.Marshal(d)
	return string(s)
}

func (d PartitionStat) String() string {
	s, _ := json.Marshal(d)
	return string(s)
}

func (d IOCountersStat) String() string {
	s, _ := json.Marshal(d)
	return string(s)
}
