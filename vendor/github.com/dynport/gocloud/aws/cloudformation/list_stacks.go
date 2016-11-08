package cloudformation

import (
	"encoding/xml"
	"strconv"
)

type ListStacksResponse struct {
	XMLName          xml.Name          `xml:"ListStacksResponse"`
	ListStacksResult *ListStacksResult `xml:"ListStacksResult"`
}

type ListStacksResult struct {
	Stacks []*Stack `xml:"StackSummaries>member"`
}

type ListStacksParameters struct {
	NextToken          string
	StackStatusFilters []string
}

func (c *Client) ListStacks(params *ListStacksParameters) (*ListStacksResponse, error) {
	r := &ListStacksResponse{}
	if params == nil {
		params = &ListStacksParameters{}
	}
	v := Values{
		"NextToken": params.NextToken,
	}
	for i, filter := range params.StackStatusFilters {
		v["StackStatusFilter.member."+strconv.Itoa(i+1)] = filter
	}
	e := c.loadCloudFormationResource("ListStacks", v, r)
	return r, e
}
