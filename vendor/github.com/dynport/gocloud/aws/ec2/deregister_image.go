package ec2

import (
	"fmt"
	"net/url"
	"strconv"
)

type DeregisterImage struct {
	ImageId string
}

func (d *DeregisterImage) Execute(client *Client) error {
	if d.ImageId == "" {
		return fmt.Errorf("ImageId must be set")
	}
	values := url.Values{
		"Version": {API_VERSIONS_EC2},
		"Action":  {"DeregisterImage"},
		"ImageId": {d.ImageId},
	}
	rsp, e := client.DoSignedRequest("GET", client.Endpoint(), values.Encode(), nil)
	if e != nil {
		return e
	}
	status := strconv.Itoa(rsp.StatusCode)
	if status[0] != '2' {
		return fmt.Errorf("expected status 2xx, got %s", status)
	}
	return nil
}
