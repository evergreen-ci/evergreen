package cloudformation

import (
	"encoding/xml"
	"time"
)

type DescribeStackEventsResponse struct {
	XMLName                   xml.Name `xml:"DescribeStackEventsResponse"`
	DescribeStackEventsResult *DescribeStackEventsResult
}

type DescribeStackEventsResult struct {
	NextToken   string        `xml:"NextToken"`
	StackEvents []*StackEvent `xml:"StackEvents>member"`
}

type StackEvent struct {
	EventId              string    `xml:"EventId"`              // Event-1-Id
	StackId              string    `xml:"StackId"`              // arn:aws:cloudformation:us-east-1:123456789:stack/MyStack/aaf549a0-a413-11df-adb3-5081b3858e83
	StackName            string    `xml:"StackName"`            // MyStack
	LogicalResourceId    string    `xml:"LogicalResourceId"`    // MyStack
	PhysicalResourceId   string    `xml:"PhysicalResourceId"`   // MyStack_One
	ResourceType         string    `xml:"ResourceType"`         // AWS::CloudFormation::Stack
	Timestamp            time.Time `xml:"Timestamp"`            // 2010-07-27T22:26:28Z
	ResourceStatus       string    `xml:"ResourceStatus"`       // CREATE_IN_PROGRESS
	ResourceStatusReason string    `xml:"ResourceStatusReason"` // User initiated
}

type DescribeStackEventsParameters struct {
	NextToken string
	StackName string
}

func (c *Client) DescribeStackEvents(params *DescribeStackEventsParameters) (*DescribeStackEventsResponse, error) {
	if params == nil {
		params = &DescribeStackEventsParameters{}
	}
	v := Values{
		"NextToken": params.NextToken,
		"StackName": params.StackName,
	}
	r := &DescribeStackEventsResponse{}
	e := c.loadCloudFormationResource("DescribeStackEvents", v, r)
	return r, e
}
