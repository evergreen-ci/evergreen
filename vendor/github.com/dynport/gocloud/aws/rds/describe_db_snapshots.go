package rds

import (
	"encoding/xml"
	"time"
)

type DescribeDBSnapshots struct {
	DBInstanceIdentifier string    `xml:",omitempty"`
	DBSnapshotIdentifier string    `xml:",omitempty"`
	Filters              []*Filter `xml:"Filters>member,omitempty"`
	Marker               string    `xml:",omitempty"`
	MaxRecords           int       `xml:",omitempty"`
	SnapshotType         string    `xml:",omitempty"`
}

type DescribeDBSnapshotsResponse struct {
	XMLName                   xml.Name                   `xml:"DescribeDBSnapshotsResponse"`
	DescribeDBSnapshotsResult *DescribeDBSnapshotsResult `xml:"DescribeDBSnapshotsResult"`
}

type DescribeDBSnapshotsResult struct {
	Snapshots []*DBSnapshot `xml:"DBSnapshots>DBSnapshot"`
}

type Filter struct {
	Name   string   `xml:",omitempty"`
	Values []string `xml:",omitempty"`
}

type DBSnapshot struct {
	AllocatedStorage     int       `xml:",omitempty"`
	AvailabilityZone     string    `xml:",omitempty"`
	DBInstanceIdentifier string    `xml:",omitempty"`
	DBSnapshotIdentifier string    `xml:",omitempty"`
	Engine               string    `xml:",omitempty"`
	EngineVersion        string    `xml:",omitempty"`
	InstanceCreateTime   time.Time `xml:",omitempty"`
	Iops                 int       `xml:",omitempty"`
	LicenseModel         string    `xml:",omitempty"`
	MasterUsername       string    `xml:",omitempty"`
	OptionGroupName      string    `xml:",omitempty"`
	PercentProgress      int       `xml:",omitempty"`
	Port                 int       `xml:",omitempty"`
	SnapshotCreateTime   time.Time `xml:",omitempty"`
	SnapshotType         string    `xml:",omitempty"`
	SourceRegion         string    `xml:",omitempty"`
	Status               string    `xml:",omitempty"`
	VpcId                string    `xml:",omitempty"`
}

func (d *DescribeDBSnapshots) Execute(client *Client) (*DescribeDBSnapshotsResponse, error) {
	v := newAction("DescribeDBSnapshots")
	e := loadValues(v, d)
	if e != nil {
		return nil, e
	}
	r := &DescribeDBSnapshotsResponse{}
	e = client.loadResource("GET", client.Endpoint()+"?"+v.Encode(), nil, r)
	return r, e
}
