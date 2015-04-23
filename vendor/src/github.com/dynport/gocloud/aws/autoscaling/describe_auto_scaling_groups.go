package autoscaling

import (
	"strconv"
	"time"
)

type DescribeAutoScalingGroups struct {
	AutoScalingGroupNames []string `xml:"AutoScalingGroupNames,omitempty"`
	MaxRecords            int      `xml:"MaxRecords,omitempty"`
	NextToken             string   `xml:"NextToken,omitempty"`
}

type DescribeAutoScalingGroupsResponse struct {
	DescribeAutoScalingGroupsResult *DescribeAutoScalingGroupsResult `xml:"DescribeAutoScalingGroupsResult"`
}

type DescribeAutoScalingGroupsResult struct {
	AutoScalingGroups []*AutoScalingGroup `xml:"AutoScalingGroups>member"`
}

type AutoScalingGroup struct {
	AutoScalingGroupARN     string              `xml:"AutoScalingGroupARN,omitempty"`
	AutoScalingGroupName    string              `xml:"AutoScalingGroupName,omitempty"`
	AvailabilityZones       []string            `xml:"AvailabilityZones,omitempty"`
	CreatedTime             time.Time           `xml:"CreatedTime,omitempty"`
	DefaultCooldown         int                 `xml:"DefaultCooldown,omitempty"`
	DesiredCapacity         int                 `xml:"DesiredCapacity,omitempty"`
	EnabledMetrics          []*EnabledMetric    `xml:"EnabledMetrics,omitempty"`
	HealthCheckGracePeriod  int                 `xml:"HealthCheckGracePeriod,omitempty"`
	HealthCheckType         string              `xml:"HealthCheckType,omitempty"`
	Instances               []*Instance         `xml:"Instances>member,omitempty"`
	LaunchConfigurationName string              `xml:"LaunchConfigurationName,omitempty"`
	LoadBalancerNames       []string            `xml:"LoadBalancerNames>member,omitempty"`
	MaxSize                 int                 `xml:"MaxSize,omitempty"`
	MinSize                 int                 `xml:"MinSize,omitempty"`
	PlacementGroup          string              `xml:"PlacementGroup,omitempty"`
	Status                  string              `xml:"Status,omitempty"`
	SuspendedProcesses      []*SuspendedProcess `xml:"SuspendedProcesses,omitempty"`
	Tags                    []*Tag              `xml:"Tags,omitempty"`
	TerminationPolicies     []string            `xml:"TerminationPolicies,omitempty"`
	VPCZoneIdentifier       string              `xml:"VPCZoneIdentifier,omitempty"`
}

type Tag struct {
	Key               string `xml:"Key,omitempty"`
	PropagateAtLaunch bool   `xml:"PropagateAtLaunch,omitempty"`
	ResourceId        string `xml:"ResourceId,omitempty"`
	ResourceType      string `xml:"ResourceType,omitempty"`
	Value             string `xml:"Value,omitempty"`
}

type SuspendedProcess struct {
	ProcessName      string `xml:"ProcessName,omitempty"`
	SuspensionReason string `xml:"SuspensionReason,omitempty"`
}

type Instance struct {
	AvailabilityZone        string `xml:"AvailabilityZone,omitempty"`
	HealthStatus            string `xml:"HealthStatus,omitempty"`
	InstanceId              string `xml:"InstanceId,omitempty"`
	LaunchConfigurationName string `xml:"LaunchConfigurationName,omitempty"`
	LifecycleState          string `xml:"LifecycleState,omitempty"`
}

type EnabledMetric struct {
	Granularity string `xml:"Granularity,omitempty"`
	Metric      string `xml:"Metric,omitempty"`
}

func (action *DescribeAutoScalingGroups) Execute(client *Client) (*DescribeAutoScalingGroupsResponse, error) {
	rsp := &DescribeAutoScalingGroupsResponse{}
	e := client.Load("GET", action.query(), rsp)
	return rsp, e
}

func (action *DescribeAutoScalingGroups) query() string {
	values := Values{
		"NextToken": action.NextToken,
		"Action":    "DescribeAutoScalingGroups",
		"Version":   "2011-01-01",
	}
	if action.MaxRecords > 0 {
		values["MaxRecords"] = strconv.Itoa(action.MaxRecords)
	}
	for i, name := range action.AutoScalingGroupNames {
		values["AutoScalingGroupNames.member."+strconv.Itoa(i+1)] = name
	}
	return values.query()
}
