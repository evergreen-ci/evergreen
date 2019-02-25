package distroqueue

import (
	"time"
)

type TaskGroupInfo struct {
	Name                  string        `bson:"name" json:"name"`
	Count                 int           `bson:"count" json:"count"`
	MaxHosts              int           `bson:"max_hosts" json:"max_hosts"`
	ExpectedDuration      time.Duration `bson:"expected_duration" json:"expected_duration"`
	CountOverThreshold    int           `bson:"count_over_threshold" json:"count_over_threshold"`
	DurationOverThreshold time.Duration `bson:"duration_over_threshold" json:"duration_over_threshold"`
}

type DistroQueueInfo struct {
	Length             int             `bson:"length" json:"length"`
	ExpectedDuration   time.Duration   `bson:"expected_duration" json:"expected_duration"`
	CountOverThreshold int             `bson:"count_over_threshold" json:"count_over_threshold"`
	TaskGroupInfos     []TaskGroupInfo `bson:"task_group_infos" json:"task_group_infos"`
}
