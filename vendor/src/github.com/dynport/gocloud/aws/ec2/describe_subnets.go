package ec2

import (
	"encoding/xml"
	"fmt"
	"net/url"
)

type DescribeSubnetsResponse struct {
	XMLName xml.Name  `xml:"DescribeSubnetsResponse"`
	Subnets []*Subnet `xml:"subnetSet>item"`
}

type DescribeSubnetsParameters struct {
	Filters []*Filter
}

func (client *Client) DescribeSubnets(params *DescribeSubnetsParameters) (*DescribeSubnetsResponse, error) {
	query := url.Values{
		"Action":  {"DescribeSubnets"},
		"Version": {API_VERSIONS_EC2},
	}
	applyFilters(query, params.Filters)
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), query.Encode(), nil)
	if e != nil {
		return nil, e
	}
	rsp := &DescribeSubnetsResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return nil, fmt.Errorf("%s (%s)", e.Error(), string(raw.Content))
	}
	return rsp, e
}
