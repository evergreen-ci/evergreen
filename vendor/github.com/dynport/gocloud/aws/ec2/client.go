package ec2

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/dynport/gocloud/aws"
)

func NewFromEnv() *Client {
	return &Client{
		aws.NewFromEnv(),
	}
}

type Client struct {
	*aws.Client
}

func (client *Client) Endpoint() string {
	prefix := "https://"
	if client.Client.Region != "" {
		prefix += client.Client.Region + "."
	}
	return prefix + "ec2.amazonaws.com"
}

const (
	API_VERSIONS_EC2       = "2013-08-15"
	CANONICAL_OWNER_ID     = "099720109477"
	SELF_OWNER_ID          = "self"
	UBUNTU_ALL             = "ubuntu/images/*"
	UBUNTU_PREFIX          = "ubuntu-*"
	UBUNTU_RARING_PREFIX   = "ubuntu-raring*"
	UBUNTU_TRUSTY_PREFIX   = "ubuntu-trusty*"
	UBUNTU_SAUCY_PREFIX    = "ubuntu-saucy*"
	ImagePrefixRaringAmd64 = "ubuntu-raring-13.04-amd64*"
)

type ImageFilter struct {
	Owner    string
	Name     string
	ImageIds []string
}

type ImageList []*Image

type InstanceList []*Instance

func (list ImageList) Len() int {
	return len(list)
}

func (list ImageList) Swap(a, b int) {
	list[a], list[b] = list[b], list[a]
}

func (list ImageList) Less(a, b int) bool {
	return list[a].Name > list[b].Name
}

type RunInstancesConfig struct {
	ImageId                string                    `json:",omitempty"`
	MinCount               int                       `json:",omitempty"`
	MaxCount               int                       `json:",omitempty"`
	InstanceType           string                    `json:",omitempty"`
	AvailabilityZone       string                    `json:",omitempty"`
	KeyName                string                    `json:",omitempty"`
	SecurityGroups         []string                  `json:",omitempty"`
	SubnetId               string                    `json:",omitempty"`
	NetworkInterfaces      []*CreateNetworkInterface `json:",omitempty"`
	BlockDeviceMappings    []*BlockDeviceMapping     `json:",omitempty"`
	UserData               string                    `json:",omitempty"`
	IamInstanceProfileName string                    `json:",omitempty"`
	EbsOptimized           bool                      `json:",omitempty"`
}

func (config *RunInstancesConfig) Values() (url.Values, error) {
	values := url.Values{}
	if config.MinCount == 0 {
		config.MinCount = 1
	}

	if config.MaxCount == 0 {
		config.MaxCount = 1
	}

	if config.ImageId == "" {
		return nil, fmt.Errorf("ImageId must be provided")
	}

	values.Add("MinCount", strconv.Itoa(config.MinCount))
	values.Add("MaxCount", strconv.Itoa(config.MaxCount))
	values.Add("ImageId", config.ImageId)
	if config.EbsOptimized {
		values.Add("EbsOptimized", "true")
	}

	if config.UserData != "" {
		values.Add("UserData", b64.EncodeToString([]byte(config.UserData)))
	}

	if config.IamInstanceProfileName != "" {
		values.Add("IamInstanceProfile.Name", config.IamInstanceProfileName)
	}

	if config.InstanceType != "" {
		values.Add("InstanceType", config.InstanceType)
	}

	if config.KeyName != "" {
		values.Add("KeyName", config.KeyName)
	}

	if config.AvailabilityZone != "" {
		values.Add("Placement.AvailabilityZone", config.AvailabilityZone)
	}

	if len(config.NetworkInterfaces) > 0 {
		for i, nic := range config.NetworkInterfaces {
			idx := strconv.Itoa(i)
			values.Add("NetworkInterface."+idx+".DeviceIndex", idx)
			values.Add("NetworkInterface."+idx+".AssociatePublicIpAddress", "true")
			values.Add("NetworkInterface."+idx+".SubnetId", nic.SubnetId)

			for i, sg := range nic.SecurityGroupIds {
				values.Add("NetworkInterface."+idx+".SecurityGroupId."+strconv.Itoa(i), sg)
			}
		}
	} else {
		for i, sg := range config.SecurityGroups {
			values.Add("SecurityGroupId."+strconv.Itoa(i+1), sg)
		}
		values.Add("SubnetId", config.SubnetId)
	}

	for i, bdm := range config.BlockDeviceMappings {
		prefix := fmt.Sprintf("BlockDeviceMapping.%d", i)
		if bdm.DeviceName == "" {
			return nil, fmt.Errorf("DeviceName must be set for all BlockDeviceMappings")
		}
		values.Add(prefix+".DeviceName", bdm.DeviceName)
		if ebs := bdm.Ebs; ebs != nil {
			prefix := prefix + ".Ebs"
			if ebs.VolumeSize > 0 {
				values.Add(prefix+".VolumeSize", strconv.Itoa(ebs.VolumeSize))
			}
			if ebs.Iops > 0 {
				values.Add(prefix+".Iops", strconv.Itoa(ebs.Iops))
			}
			if ebs.DeleteOnTermination {
				values.Add(prefix+".DeleteOnTermination", "true")
			}
			if ebs.Encrypted {
				values.Add(prefix+".Encrypted", "true")
			}
			if ebs.SnapshotId != "" {
				values.Add(prefix+".SnapshotId", ebs.SnapshotId)
			}
			if ebs.VolumeType != "" {
				values.Add(prefix+".VolumeType", ebs.VolumeType)
			}
		}
	}
	return values, nil
}

func (config *RunInstancesConfig) AddPublicIp() error {
	if config.SubnetId == "" {
		return fmt.Errorf("SubnetId must be set")
	}
	nic := &CreateNetworkInterface{
		DeviceIndex: len(config.NetworkInterfaces), AssociatePublicIpAddress: true, SubnetId: config.SubnetId,
		SecurityGroupIds: config.SecurityGroups,
	}
	config.NetworkInterfaces = []*CreateNetworkInterface{nic}
	return nil
}

func queryForAction(action string) string {
	values := &url.Values{}
	values.Add("Version", API_VERSIONS_EC2)
	values.Add("Action", action)
	return values.Encode()
}

func (client *Client) DescribeTags() (tags TagList, e error) {
	query := queryForAction("DescribeTags")
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), query, nil)
	if e != nil {
		return tags, e
	}
	rsp := &DescribeTagsResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return tags, e
	}
	return rsp.Tags, e
}

func (client *Client) CreateTags(resourceIds []string, tags map[string]string) error {
	values := &url.Values{}
	for i, id := range resourceIds {
		values.Add("ResourceId."+strconv.Itoa(i), id)
	}
	tagsCount := 1
	for k, v := range tags {
		prefix := fmt.Sprintf("Tag.%d.", tagsCount)
		values.Add(prefix+"Key", k)
		values.Add(prefix+"Value", v)
		tagsCount++
	}
	query := queryForAction("CreateTags") + "&" + values.Encode()
	_, e := client.DoSignedRequest("POST", client.Endpoint(), query, nil)
	if e != nil {
		return e
	}
	return nil
}

func (client *Client) TerminateInstances(ids []string) (*aws.Response, error) {
	query := queryForAction("TerminateInstances")
	for i, id := range ids {
		query += fmt.Sprintf("&InstanceId.%d=%s", i, id)
	}
	return client.DoSignedRequest("DELETE", client.Endpoint(), query, nil)
}

type Error struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

type ErrorResponse struct {
	XMLName   xml.Name `xml:"Response"`
	RequestID string   `xml:"RequestID"`
	Errors    []*Error `xml:"Errors>Error"`
}

func (er *ErrorResponse) ErrorStrings() string {
	out := []string{}
	for _, e := range er.Errors {
		out = append(out, fmt.Sprintf("%s: %s", e.Code, e.Message))
	}
	return strings.Join(out, ", ")
}

type RunInstancesResponse struct {
	XMLName       xml.Name    `xml:"RunInstancesResponse"`
	RequestId     string      `xml:"requestId"`
	ReservationId string      `xml:"reservationId"`
	OwnerId       string      `xml:"ownerId"`
	Instances     []*Instance `xml:"instancesSet>item"`
}

var b64 = base64.StdEncoding

func (client *Client) RunInstances(config *RunInstancesConfig) (list InstanceList, e error) {
	values, e := config.Values()
	if e != nil {
		return nil, e
	}
	query := queryForAction("RunInstances") + "&" + values.Encode()

	raw, e := client.DoSignedRequest("POST", client.Endpoint(), query, nil)
	if e != nil {
		return list, e
	}
	er := &ErrorResponse{}
	if e := xml.Unmarshal(raw.Content, er); e == nil {
		return nil, fmt.Errorf(er.ErrorStrings())
	}
	rsp := &RunInstancesResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return list, e
	}
	return InstanceList(rsp.Instances), nil
}

type DescribeInstancesOptions struct {
	InstanceIds []string
	Filters     []*Filter
}

func (client *Client) DescribeInstancesWithOptions(options *DescribeInstancesOptions) (instances []*Instance, e error) {
	if options == nil {
		options = &DescribeInstancesOptions{}
	}
	values := url.Values{"Version": {API_VERSIONS_EC2}, "Action": {"DescribeInstances"}}
	if len(options.InstanceIds) > 0 {
		for i, id := range options.InstanceIds {
			values.Add("InstanceId."+strconv.Itoa(i+1), id)
		}
	}
	applyFilters(values, options.Filters)
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), values.Encode(), nil)
	if e != nil {
		return instances, e
	}
	rsp := &DescribeInstancesResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		e = fmt.Errorf("%s: %s", e.Error(), string(raw.Content))
		return instances, e
	}
	return rsp.Instances(), nil
}

func (client *Client) DescribeInstances() (instances []*Instance, e error) {
	return client.DescribeInstancesWithOptions(nil)
}
