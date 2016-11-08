package autoscaling

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/dynport/gocloud/aws"
)

type DescribeScalingActivities struct {
	ActivityIds          []string
	AutoScalingGroupName string
	MaxRecords           string
	NextToken            string
}

type Values map[string]string

func (v Values) query() string {
	out := url.Values{}
	for k, v := range v {
		if v != "" {
			out[k] = []string{v}
		}
	}
	if len(out) > 0 {
		return "?" + out.Encode()
	}
	return ""
}

func NewFromEnv() *Client {
	return &Client{
		Client: aws.NewFromEnv(),
	}
}

type Client struct {
	*aws.Client
}

func (d *DescribeScalingActivities) Execute(client *Client) (*DescribeScalingActivitiesResponse, error) {
	ep, e := client.Endpoint()
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequest("GET", ep+d.query(), nil)

	if e != nil {
		return nil, e
	}
	now := time.Now()
	client.SignAwsRequestV2(req, now)
	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return nil, e
	}
	if rsp.Status[0] != '2' {
		return nil, fmt.Errorf("expected status 2xx, got %s. %s", rsp.Status, string(b))
	}
	dsa := &DescribeScalingActivitiesResponse{}
	e = xml.Unmarshal(b, dsa)
	if e != nil {
		return nil, fmt.Errorf(e.Error() + ": " + string(b))
	}
	return dsa, e
}

func (d *DescribeScalingActivities) query() string {
	values := Values{
		"NextToken":            d.NextToken,
		"MaxRecords":           d.MaxRecords,
		"AutoScalingGroupName": d.AutoScalingGroupName,
		"Action":               "DescribeScalingActivities",
		"Version":              "2011-01-01",
	}
	for i, id := range d.ActivityIds {
		values["ActivityIds.member."+strconv.Itoa(i+1)] = id
	}
	return values.query()
}

type DescribeScalingActivitiesResponse struct {
	XMLName                         xml.Name                         `xml:"DescribeScalingActivitiesResponse"`
	DescribeScalingActivitiesResult *DescribeScalingActivitiesResult `xml:"DescribeScalingActivitiesResult"`
}

type DescribeScalingActivitiesResult struct {
	Activities []*Activity `xml:"Activities>member"`
}

type Activity struct {
	StatusCode           string    `xml:"StatusCode"`           // Failed</StatusCode>
	Progress             int       `xml:"Progress"`             // 0</Progress>
	ActivityId           string    `xml:"ActivityId"`           // 063308ae-aa22-4a9b-94f4-9faeEXAMPLE</ActivityId>
	StartTime            time.Time `xml:"StartTime"`            // 2012-04-12T17:32:07.882Z</StartTime>
	AutoScalingGroupName string    `xml:"AutoScalingGroupName"` // my-test-asg</AutoScalingGroupName>
	Cause                string    `xml:"Cause"`                // At 2012-04-12T17:31:30Z a user request created an AutoScalingGroup changing the desired capacity from 0 to 1.  At 2012-04-12T17:32:07Z an instance was started in response to a difference between desired and actual capacity, increasing the capacity from 0 to 1.</Cause>
	Details              string    `xml:"Details"`              // {}</Details>
	Description          string    `xml:"Description"`          // Launching a new EC2 instance.  Status Reason: The image id 'ami-4edb0327' does not exist. Launching EC2 instance failed.</Description>
	EndTime              time.Time `xml:"EndTime"`              // 2012-04-12T17:32:08Z</EndTime>
	StatusMessage        string    `xml:"StatusMessage"`        // The image id 'ami-4edb0327' does not exist. Launching EC2 instance failed.</StatusMessage>
}
