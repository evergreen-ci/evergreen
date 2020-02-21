package load

import (
	"encoding/json"

	"github.com/shirou/gopsutil/internal/common"
)

var invoke common.Invoker = common.Invoke{}

type AvgStat struct {
	Load1  float64 `json:"load1" bson:"load1,omitempty"`
	Load5  float64 `json:"load5" bson:"load5,omitempty"`
	Load15 float64 `json:"load15" bson:"load15,omitempty"`
}

func (l AvgStat) String() string {
	s, _ := json.Marshal(l)
	return string(s)
}

type MiscStat struct {
	ProcsTotal   int `json:"procsTotal" bson:"procsTotal,omitempty"`
	ProcsRunning int `json:"procsRunning" bson:"procsRunning,omitempty"`
	ProcsBlocked int `json:"procsBlocked" bson:"procsBlocked,omitempty"`
	Ctxt         int `json:"ctxt" bson:"ctxt,omitempty"`
}

func (m MiscStat) String() string {
	s, _ := json.Marshal(m)
	return string(s)
}
