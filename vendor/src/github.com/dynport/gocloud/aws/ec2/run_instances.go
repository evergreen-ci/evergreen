package ec2

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
)

type RunInstances struct {
	InstanceType                      string
	ImageId                           string
	MinCount                          int
	MaxCount                          int
	KeyName                           string
	SecurityGroupIds                  []string
	SubnetId                          string
	UserData                          string
	AvailabilityZone                  string
	InstanceInitiatedShutdownBehavior string
	IamInstanceProfileName            string
	BlockDeviceMapings                []*BlockDeviceMapping

	NetworkInterfaces []*CreateNetworkInterface
}

func (config *RunInstances) AddPublicIp() {
	nic := &CreateNetworkInterface{
		DeviceIndex: len(config.NetworkInterfaces), AssociatePublicIpAddress: true, SubnetId: config.SubnetId,
		SecurityGroupIds: config.SecurityGroupIds,
	}
	config.NetworkInterfaces = []*CreateNetworkInterface{nic}
}

type queryParams map[string]string

func (q queryParams) Add(key, value string) {
	q[key] = value
}

func (q queryParams) Encode() string {
	values := url.Values{}
	for k, v := range q {
		if v != "" {
			values.Add(k, v)
		}
	}
	return values.Encode()
}

func (action *RunInstances) Execute(client *Client) (*RunInstancesResponse, error) {
	if action.MinCount < 1 {
		action.MinCount = 1
	}
	if action.MaxCount < 1 {
		action.MaxCount = 1
	}
	values := queryParams{
		"Version":                 API_VERSIONS_EC2,
		"Action":                  "RunInstances",
		"MinCount":                strconv.Itoa(action.MinCount),
		"MaxCount":                strconv.Itoa(action.MaxCount),
		"ImageId":                 action.ImageId,
		"KeyName":                 action.KeyName,
		"InstanceType":            action.InstanceType,
		"UserData":                b64.EncodeToString([]byte(action.UserData)),
		"AvailabilityZone":        action.AvailabilityZone,
		"IamInstanceProfile.Name": action.IamInstanceProfileName,
	}
	if len(action.NetworkInterfaces) > 0 {
		for i, nic := range action.NetworkInterfaces {
			idx := strconv.Itoa(i)
			values.Add("NetworkInterface."+idx+".DeviceIndex", idx)
			values.Add("NetworkInterface."+idx+".AssociatePublicIpAddress", "true")
			values.Add("NetworkInterface."+idx+".SubnetId", nic.SubnetId)

			for i, sg := range nic.SecurityGroupIds {
				values.Add("NetworkInterface."+idx+".SecurityGroupId."+strconv.Itoa(i), sg)
			}
		}
	} else {
		values.Add("SubnetId", action.SubnetId)
		for i, sg := range action.SecurityGroupIds {
			values.Add("SecurityGroupId."+strconv.Itoa(i+1), sg)
		}
	}
	raw, e := client.DoSignedRequest("POST", client.Endpoint(), values.Encode(), nil)
	if e != nil {
		return nil, e
	}
	er := &ErrorResponse{}
	if e := xml.Unmarshal(raw.Content, er); e == nil {
		return nil, fmt.Errorf(er.ErrorStrings())
	}
	rsp := &RunInstancesResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	return rsp, e
}
