package ec2

import (
	"fmt"
	"net/url"
)

type Subnet struct {
	SubnetId                string `xml:"subnetId"`
	State                   string `xml:"state"`
	VpcId                   string `xml:"vpcId"`
	CidrBlock               string `xml:"cidrBlock"`
	AvailableIpAddressCount int    `xml:"availableIpAddressCount"`
	AvailabilityZone        string `xml:"availabilityZone"`
	DefaultForAz            bool   `xml:"defaultForAz"`
	MapPublicIpOnLaunch     bool   `xml:"mapPublicIpOnLaunch"`
}

type Filter struct {
	Name   string
	Values []string
}

func applyFilters(values url.Values, filters []*Filter) {
	for n, filter := range filters {
		key := fmt.Sprintf("Filter.%d.Name", n+1)
		for m, value := range filter.Values {
			valueKey := fmt.Sprintf("Filter.%d.Value.%d", n+1, m+1)
			values.Add(key, filter.Name)
			values.Add(valueKey, value)
		}
	}
}

type DescribeSubnetsOptions struct {
	SubnetIds []string
	Filters   []*Filter
}
