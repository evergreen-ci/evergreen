package autoscaling

import (
	"encoding/xml"
	"strconv"
	"time"
)

func (action *DescribeLaunchConfigurations) Execute(client *Client) (*DescribeLaunchConfigurationsResponse, error) {
	rsp := &DescribeLaunchConfigurationsResponse{}
	e := client.Load("GET", action.query(), rsp)
	return rsp, e
}

func (action *DescribeLaunchConfigurations) query() string {
	values := Values{
		"NextToken": action.NextToken,
		"Action":    "DescribeLaunchConfigurations",
		"Version":   "2011-01-01",
	}
	if action.MaxRecords > 0 {
		values["MaxRecords"] = strconv.Itoa(action.MaxRecords)
	}
	for i, name := range action.LaunchConfigurationNames {
		values["LaunchConfigurationNames.member."+strconv.Itoa(i+1)] = name
	}
	return values.query()
}

type DescribeLaunchConfigurations struct {
	LaunchConfigurationNames []string `xml:",omitempty"`
	MaxRecords               int      `xml:",omitempty"`
	NextToken                string   `xml:",omitempty"`
}

type DescribeLaunchConfigurationsResponse struct {
	XMLName                            xml.Name                            `xml:"DescribeLaunchConfigurationsResponse"`
	DescribeLaunchConfigurationsResult *DescribeLaunchConfigurationsResult `xml:"DescribeLaunchConfigurationsResult,omitempty"`
}

type DescribeLaunchConfigurationsResult struct {
	XMLName              xml.Name               `xml:"DescribeLaunchConfigurationsResult"`
	LaunchConfigurations []*LaunchConfiguration `xml:"LaunchConfigurations>member,omitempty"`
	NextToken            string                 `xml:",omitempty"`
}

type LaunchConfiguration struct {
	AssociatePublicIpAddress bool                  `xml:",omitempty"`
	BlockDeviceMappings      []*BlockDeviceMapping `xml:",omitempty"`
	CreatedTime              time.Time             `xml:",omitempty"`
	EbsOptimized             bool                  `xml:",omitempty"`
	IamInstanceProfile       string                `xml:",omitempty"`
	ImageId                  string                `xml:",omitempty"`
	InstanceMonitoring       *InstanceMonitoring   `xml:",omitempty"`
	InstanceType             string                `xml:",omitempty"`
	KernelId                 string                `xml:",omitempty"`
	KeyName                  string                `xml:",omitempty"`
	LaunchConfigurationARN   string                `xml:",omitempty"`
	LaunchConfigurationName  string                `xml:",omitempty"`
	PlacementTenancy         string                `xml:",omitempty"`
	RamdiskId                string                `xml:",omitempty"`
	SecurityGroups           []string              `xml:"SecurityGroups>member,omitempty"`
	SpotPrice                string                `xml:",omitempty"`
	UserData                 string                `xml:",omitempty"`
}

type BlockDeviceMapping struct {
	DeviceName  string `xml:",omitempty"`
	Ebs         *Ebs   `xml:",omitempty"`
	NoDevice    bool   `xml:",omitempty"`
	VirtualName string `xml:",omitempty"`
}

type Ebs struct {
	DeleteOnTermination bool   `xml:",omitempty"`
	Iops                int    `xml:",omitempty"`
	SnapshotId          string `xml:",omitempty"`
	VolumeSize          int    `xml:",omitempty"`
	VolumeType          string `xml:",omitempty"`
}

type InstanceMonitoring struct {
	Enabled bool `xml:",omitempty"`
}
