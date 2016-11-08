package ec2

import (
	"encoding/xml"
	"net/url"
	"strconv"
)

type DescribeInstances struct {
	InstanceIds []string
	Filters     []*Filter
}

func (action *DescribeInstances) Execute(client *Client) (*DescribeInstancesResponse, error) {
	values := url.Values{"Version": {API_VERSIONS_EC2}, "Action": {"DescribeInstances"}}
	if len(action.InstanceIds) > 0 {
		for i, id := range action.InstanceIds {
			values.Add("InstanceId."+strconv.Itoa(i+1), id)
		}
	}
	applyFilters(values, action.Filters)
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), values.Encode(), nil)
	if e != nil {
		return nil, e
	}
	rsp := &DescribeInstancesResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	return rsp, e
}
