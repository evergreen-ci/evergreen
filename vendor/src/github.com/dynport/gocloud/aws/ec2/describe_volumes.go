package ec2

import (
	"encoding/xml"
	"net/url"
	"strconv"
	"time"
)

type DescribeVolumes struct {
	VolumeIds []string
	Filters   []*Filter
}

type DescribeVolumesResponse struct {
	XMLName xml.Name  `xml:"DescribeVolumesResponse"`
	Volumes []*Volume `xml:"volumeSet>item"`
}

type Volume struct {
	VolumeId         string    `xml:"volumeId"`         // vol-1a2b3c4d</volumeId>
	Size             string    `xml:"size"`             // 80</size>
	SnapshotId       string    `xml:"snapshotId/"`      //
	AvailabilityZone string    `xml:"availabilityZone"` // us-east-1a</availabilityZone>
	Status           string    `xml:"status"`           // in-use</status>
	CreateTime       time.Time `xml:"createTime"`       // YYYY-MM-DDTHH:MM:SS.SSSZ</createTime>

	Attachments []*Attachment `xml:"attachmentSet>item"`
}

func (action *DescribeVolumes) Execute(client *Client) (*DescribeVolumesResponse, error) {
	values := url.Values{
		"Version": {API_VERSIONS_EC2},
		"Action":  {"DescribeVolumes"},
	}
	for i, v := range action.VolumeIds {
		values.Set("VolumeId."+strconv.Itoa(i+1), v)
	}
	rsp, e := client.DoSignedRequest("GET", client.Endpoint(), values.Encode(), nil)
	if e != nil {
		return nil, e
	}
	dvr := &DescribeVolumesResponse{}
	e = xml.Unmarshal(rsp.Content, dvr)
	return dvr, e
}

type Attachment struct {
	VolumeId            string `xml:"volumeId"`            // vol-1a2b3c4d</volumeId>
	InstanceId          string `xml:"instanceId"`          // i-1a2b3c4d</instanceId>
	Device              string `xml:"device"`              // /dev/sdh</device>
	Status              string `xml:"status"`              // attached</status>
	AttachTime          string `xml:"attachTime"`          // YYYY-MM-DDTHH:MM:SS.SSSZ</attachTime>
	DeleteOnTermination string `xml:"deleteOnTermination"` // false</deleteOnTermination>
}
