package cloudformation

import (
	"encoding/xml"
	"time"
)

type DescribeStackResourcesResponse struct {
	XMLName                      xml.Name                      `xml:"DescribeStackResourcesResponse"`
	DescribeStackResourcesResult *DescribeStackResourcesResult `xml:"DescribeStackResourcesResult"`
}

type DescribeStackResourcesResult struct {
	StackResources []*StackResource `xml:"StackResources>member"`
}

type StackResource struct {
	StackId            string    //arn:aws:cloudformation:us-east-1:123456789:stack/MyStack/aaf549a0-a413-11df-adb3-5081b3858e83</StackId>
	StackName          string    //MyStack</StackName>
	LogicalResourceId  string    //MyDBInstance</LogicalResourceId>
	PhysicalResourceId string    //MyStack_DB1</PhysicalResourceId>
	ResourceType       string    //AWS::DBInstance</ResourceType>
	Timestamp          time.Time //2010-07-27T22:27:28Z</Timestamp>
	ResourceStatus     string    //CREATE_COMPLETE</ResourceStatus>
}

type DescribeStackResourcesParameters struct {
	LogicalResourceId  string
	PhysicalResourceId string
	StackName          string
}

func (client *Client) DescribeStackResources(params DescribeStackResourcesParameters) (*DescribeStackResourcesResponse, error) {
	r := &DescribeStackResourcesResponse{}
	values := Values{
		"StackName":          params.StackName,
		"PhysicalResourceId": params.PhysicalResourceId,
		"LogicalResourceId":  params.LogicalResourceId,
	}
	e := client.loadCloudFormationResource("DescribeStackResources", values, r)
	return r, e
}
