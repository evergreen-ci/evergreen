package ec2

import (
	"encoding/xml"
	"net/url"
	"strconv"
)

type DescribeImages struct {
	ExecutableBys []string
	ImageIds      []string
	Owners        []string
	Filters       []*Filter
}

func (action *DescribeImages) Execute(client *Client) (*DescribeImagesResponse, error) {
	values := url.Values{
		"Version": {API_VERSIONS_EC2},
		"Action":  {"DescribeImages"},
	}

	for i, v := range action.Owners {
		values.Set("Owner."+strconv.Itoa(i+1), v)
	}
	for i, v := range action.ImageIds {
		values.Set("ImageId."+strconv.Itoa(i+1), v)
	}
	for i, filter := range action.Filters {
		values.Set("Filter."+strconv.Itoa(i+1)+".Name", filter.Name)
		for j, value := range filter.Values {
			values.Set("Filter."+strconv.Itoa(i+1)+"."+strconv.Itoa(j+1), value)
		}
	}
	rsp, e := client.DoSignedRequest("GET", client.Endpoint(), values.Encode(), nil)
	if e != nil {
		return nil, e
	}
	dvr := &DescribeImagesResponse{}
	e = xml.Unmarshal(rsp.Content, dvr)
	return dvr, e
}
