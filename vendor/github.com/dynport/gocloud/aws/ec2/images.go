package ec2

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
)

func (client *Client) DescribeImages() (images []*Image, e error) {
	return client.DescribeImagesWithFilter(&ImageFilter{})
}

func (client *Client) DescribeImagesWithFilter(filter *ImageFilter) (images ImageList, e error) {
	values := url.Values{
		"Version": {API_VERSIONS_EC2},
		"Action":  {"DescribeImages"},
	}
	if filter.Owner != "" {
		values.Add("Owner.1", filter.Owner)
	}
	if filter.Name != "" {
		values.Add("Filter.1.Name", "name")
		values.Add("Filter.1.Value.0", filter.Name)
	}
	for i, id := range filter.ImageIds {
		values.Add("ImageId."+strconv.Itoa(i+1), id)
	}
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), values.Encode(), nil)
	if e != nil {
		return
	}
	rsp := &DescribeImagesResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return images, e
	}
	return rsp.Images, nil
}

type CreateImageOptions struct {
	InstanceId  string // required
	Name        string // required
	Description string
	NoReboot    bool
}

type CreateImageResponse struct {
	XMLName   xml.Name `xml:"CreateImageResponse"`
	RequestId string   `xml:"requestId"`
	ImageId   string   `xml:"imageId"`
}

func (client *Client) CreateImage(opts *CreateImageOptions) (rsp *CreateImageResponse, e error) {
	if opts.InstanceId == "" {
		return nil, fmt.Errorf("InstanceId must be provided")
	}
	if opts.Name == "" {
		return nil, fmt.Errorf("InstanceId must be provided")
	}
	values := &url.Values{}
	values.Add("Version", API_VERSIONS_EC2)
	values.Add("Action", "CreateImage")
	values.Add("Name", opts.Name)
	values.Add("InstanceId", opts.InstanceId)
	if opts.Description != "" {
		values.Add("Description", opts.Description)
	}
	if opts.NoReboot {
		values.Add("NoReboot", "true")
	}

	raw, e := client.DoSignedRequest("GET", client.Endpoint(), values.Encode(), nil)
	if e != nil {
		return nil, e
	}
	rsp = &CreateImageResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	return rsp, e
}
