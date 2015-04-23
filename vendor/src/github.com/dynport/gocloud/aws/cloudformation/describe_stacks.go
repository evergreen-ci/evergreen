package cloudformation

import (
	"encoding/xml"
	"time"
)

type DescribeStacksResponse struct {
	XMLName              xml.Name              `xml:"DescribeStacksResponse"`
	DescribeStacksResult *DescribeStacksResult `xml:"DescribeStacksResult"`
}

type DescribeStacksResult struct {
	Stacks []*Stack `xml:"Stacks>member"`
}

type DescribeStacksParameters struct {
	NextToken string
	StackName string
}

type DescribeStacks struct {
	NextToken string
	StackName string
}

func (a *DescribeStacks) Execute(client *Client) (*DescribeStacksResponse, error) {
	r := &DescribeStacksResponse{}
	v := Values{
		"NextToken": a.NextToken,
		"StackName": a.StackName,
	}
	e := client.loadCloudFormationResource("DescribeStacks", v, r)
	return r, e
}

func (client *Client) DescribeStacks(params *DescribeStacksParameters) (rsp *DescribeStacksResponse, e error) {
	action := &DescribeStacks{}
	if params != nil {
		action.NextToken = params.NextToken
		action.StackName = params.StackName
	}
	return action.Execute(client)
}

type Stack struct {
	StackName           string            `xml:"StackName"`       // MyStack</StackName>
	StackId             string            `xml:"StackId"`         // arn:aws:cloudformation:us-east-1:123456789:stack/MyStack/aaf549a0-a413-11df-adb3-5081b3858e83</StackId>
	CreationTime        time.Time         `xml:"CreationTime"`    // 2010-07-27T22:28:28Z</CreationTime>
	StackStatus         string            `xml:"StackStatus"`     // CREATE_COMPLETE</StackStatus>
	DisableRollback     bool              `xml:"DisableRollback"` // false</DisableRollback>
	TemplateDescription string            `xml:"TemplateDescription"`
	Outputs             []*Output         `xml:"Outputs>member"`
	Parameters          []*StackParameter `xml:"Parameters>member"`
}

type Output struct {
	OutputKey   string `xml:"OutputKey"`
	OutputValue string `xml:"OutputValue"`
}
