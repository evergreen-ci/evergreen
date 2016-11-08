package rds

import (
	"encoding/xml"
	"time"
)

type DescribeDBInstancesResponse struct {
	XMLName                   xml.Name                   `xml:"DescribeDBInstancesResponse"`
	DescribeDBInstancesResult *DescribeDBInstancesResult `xml:"DescribeDBInstancesResult"`
}

type DescribeDBInstancesResult struct {
	Instances []*DBInstance `xml:"DBInstances>DBInstance"`
}

type Endpoint struct {
	Port    string `xml:"Port"`
	Address string `xml:"Address"`
}

type VpcSecurityGroupMembership struct {
	Status             string `xml:"Status"`
	VpcSecurityGroupId string `xml:"VpcSecurityGroupId"`
}

type DBSecurityGroup struct {
	Status              string `xml:"Status"`
	DBSecurityGroupName string `xml:"DBSecurityGroupName"`
}

type DBInstance struct {
	LatestRestorableTime       string                        `xml:"LatestRestorableTime"`
	Engine                     string                        `xml:"Engine"`
	PendingModifiedValues      interface{}                   `xml:"PendingModifiedValues"`
	BackupRetentionPeriod      string                        `xml:"BackupRetentionPeriod"`
	MultiAZ                    bool                          `xml:"MultiAZ"`
	LicenseModel               string                        `xml:"LicenseModel"`
	DBInstanceStatus           string                        `xml:"DBInstanceStatus"`
	EngineVersion              string                        `xml:"EngineVersion"`
	Endpoint                   *Endpoint                     `xml:"Endpoint"`
	DBInstanceIdentifier       string                        `xml:"DBInstanceIdentifier"`
	VpcSecurityGroups          []*VpcSecurityGroupMembership `xml:"VpcSecurityGroups"`
	DBSecurityGroups           []*DBSecurityGroup            `xml:"DBSecurityGroups"`
	PreferredBackupWindow      string                        `xml:"PreferredBackupWindow"`
	AutoMinorVersionUpgrade    bool                          `xml:"AutoMinorVersionUpgrade"`
	PreferredMaintenanceWindow string                        `xml:"PreferredMaintenanceWindow"`
	AvailabilityZone           string                        `xml:"AvailabilityZone"`
	InstanceCreateTime         time.Time                     `xml:"InstanceCreateTime"`
	AllocatedStorage           int                           `xml:"AllocatedStorage"`
	DBInstanceClass            string                        `xml:"DBInstanceClass"`
	MasterUsername             string                        `xml:"MasterUsername"`
}

func (d *DescribeDBInstances) Execute(client *Client) (*DescribeDBInstancesResponse, error) {
	v := newAction("DescribeDBInstances")
	e := loadValues(v, d)
	if e != nil {
		return nil, e
	}
	r := &DescribeDBInstancesResponse{}
	e = client.loadResource("GET", client.Endpoint()+"?"+v.Encode(), nil, r)
	return r, e
}
