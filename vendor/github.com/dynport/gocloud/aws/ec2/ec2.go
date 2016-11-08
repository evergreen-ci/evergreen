package ec2

import (
	"encoding/xml"
	"fmt"
	"time"
)

type KeyPair struct {
	KeyName        string `xml:"keyName"`
	KeyFingerprint string `xml:"keyFingerprint"`
}

type DescribeKeyPairsResponse struct {
	KeyPairs []*KeyPair `xml:"keySet>item"`
}

func (client *Client) DescribeKeyPairs() (pairs []*KeyPair, e error) {
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), queryForAction("DescribeKeyPairs"), nil)
	if e != nil {
		return pairs, e
	}
	rsp := &DescribeKeyPairsResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return pairs, e
	}
	return rsp.KeyPairs, nil

}

type DescribeImagesResponse struct {
	XMLName   xml.Name `xml:"DescribeImagesResponse"`
	RequestId string   `xml:"requestId"`
	Images    []*Image `xml:"imagesSet>item"`
}

type Image struct {
	ImageId             string                `xml:"imageId"`
	ImageLocation       string                `xml:"imageLocation"`
	ImageState          string                `xml:"imageState"`
	ImageOwnerId        string                `xml:"imageOwnerId"`
	IsPublic            bool                  `xml:"isPublic"`
	Architecture        string                `xml:"architecture"`
	ImageType           string                `xml:"imageType"`
	ImageOwnerAlias     string                `xml:"imageOwnerAlias"`
	Name                string                `xml:"name"`
	RootDeviceType      string                `xml:"rootDeviceType"`
	VirtualizationType  string                `xml:"virtualizationType"`
	Hypervisor          string                `xml:"hypervisor"`
	BlockDeviceMappings []*BlockDeviceMapping `xml:"blockDeviceMapping>item"`
	ProductCodes        []*ProductCode        `xml:"productCodes>item"`
	Tags                []*Tag                `xml:"tagSet>item"`
}

type ProductCode struct {
	ProductCode string `xml:"productCode"`
	Type        string `xml:"type"`
}

type Instance struct {
	InstanceId                string    `xml:"instanceId"`
	ImageId                   string    `xml:"imageId"`
	InstanceStateCode         int       `xml:"instanceState>code"`
	InstanceStateName         string    `xml:"instanceState>name"`
	PrivateDnsName            string    `xml:"privateDnsName"`
	DnsName                   string    `xml:"dnsName"`
	Reason                    string    `xml:"reason"`
	KeyName                   string    `xml:"keyName"`
	AmiLaunchIndex            int       `xml:"amiLaunchIndex"`
	InstanceType              string    `xml:"instanceType"`
	LaunchTime                time.Time `xml:"launchTime"`
	PlacementAvailabilityZone string    `xml:"placement>availabilityZone"`
	PlacementTenancy          string    `xml:"placement>tenancy"`
	KernelId                  string    `xml:"kernelId"`
	MonitoringState           string    `xml:"monitoring>state"`
	SubnetId                  string    `xml:"subnetId"`
	VpcId                     string    `xml:"vpcId"`
	PrivateIpAddress          string    `xml:"privateIpAddress"`
	IpAddress                 string    `xml:"ipAddress"`
	SourceDestCheck           string    `xml:"sourceDestCheck"`
	Architecture              string    `xml:"architecture"`
	RootDeviceType            string    `xml:"rootDeviceType"`
	RootDeviceName            string    `xml:"rootDeviceName"`
	VirtualizationType        string    `xml:"virtualizationType"`
	ClientToken               string    `xml:"clientToken"`
	Hypervisor                string    `xml:"hypervisor"`
	EbsOptimized              string    `xml:"ebsOptimized"`

	BlockDeviceMappings []*BlockDeviceMapping `xml:"blockDeviceMapping>item"`
	SecurityGroups      []*SecurityGroup      `xml:"groupSet>item"`
	Tags                []*Tag                `xml:"tagSet>item"`
	NetworkInterfaces   []*NetworkInterface   `xml:"networkInterfaceSet>item"`
}

func (instance *Instance) Name() string {
	for _, tag := range instance.Tags {
		if tag.Key == "Name" {
			return tag.Value
		}
	}
	return ""
}

type NetworkInterface struct {
	NetworkInterfaceId            string           `xml:"networkInterfaceId"`
	SubnetId                      string           `xml:"subnetId"`
	VpcId                         string           `xml:"vpcId"`
	Description                   string           `xml:"description"`
	OwnerId                       string           `xml:"ownerId"`
	Status                        string           `xml:"status"`
	MacAddress                    string           `xml:"macAddress"`
	PrivateIpAddress              string           `xml:"privateIpAddress"`
	PrivateDnsName                string           `xml:"privateDnsName"`
	SourceDestCheck               bool             `xml:"sourceDestCheck"`
	SecurityGroups                []*SecurityGroup `xml:"groupSet>item"`
	AttachmentAttachmentId        string           `xml:"attachment>attachmentId"`
	AttachmentDeviceIndex         int              `xml:"attachment>deviceIndex"`
	AttachmentStatus              string           `xml:"attachment>status"`
	AttachmentAttachTime          time.Time        `xml:"attachment>attachTime"`
	AttachmentDeleteOnTermination bool             `xml:"attachment>deleteOnTermination"`
	AssociationPublicIp           string           `xml:"association>publicIp"`
	AssociationPublicDnsName      string           `xml:"association>publicDnsName"`
	AssociationIpOwnerId          string           `xml:"association>ipOwnerId"`

	PrivateIpAddresses []*IpAddress `xml:"privateIpAddressesSet>item"`
}

type IpAddress struct {
	PrivateIpAddress string `xml:"privateIpAddress"`
	PrivateDnsName   string `xml:"privateDnsName"`
	Primary          bool   `xml:"primary"`
	PublicIp         string `xml:"publicIp"`
	PublicDnsName    string `xml:"publicDnsName"`
	IpOwnerId        string `xml:"ipOwnerId"`
}

type TagList []*Tag

func (list TagList) Len() int {
	return len(list)
}

func (list TagList) Swap(a, b int) {
	list[a], list[b] = list[b], list[a]
}

func (list TagList) Less(a, b int) bool {
	return list[a].String() < list[b].String()
}

func (tag *Tag) String() string {
	return fmt.Sprintf("%s %s %s %s", tag.ResourceType, tag.ResourceId, tag.Key, tag.Value)
}

type Tag struct {
	Key          string `xml:"key,omitempty"`
	Value        string `xml:"value,omitempty"`
	ResourceId   string `xml:"resourceId,omitempty"`
	ResourceType string `xml:"resourceType,omitempty"`
}

type BlockDeviceMapping struct {
	DeviceName string `xml:"deviceName,omitempty" json:",omitempty"`
	Ebs        *Ebs   `xml:"ebs,omitempty" json:",omitempty"`
}

const (
	VolumeTypeGp       = "gp2"
	VolumeTypeIo1      = "io1"
	VolumeTypeStandard = "standard"
)

type Ebs struct {
	SnapshotId          string `xml:"snapshotId,omitempty" json:",omitempty"`
	VolumeSize          int    `xml:"volumeSize,omitempty" json:",omitempty"`
	DeleteOnTermination bool   `xml:"deleteOnTermination,omitempty" json:",omitempty"`
	VolumeType          string `xml:"volumeType,omitempty json:",omitempty""` // see VolumeType... (e.g. gp, io1, standard)
	Iops                int    `xml:"iops,omitempty" json:",omitempty"`
	Encrypted           bool   `xml:"encrypted,omitempty" json:",omitempty"`
}

type Reservation struct {
	ReservationId string      `xml:"reservationId"`
	OwnerId       string      `xml:"ownerId"`
	Instances     []*Instance `xml:"instancesSet>item"`
}

type DescribeInstancesResponse struct {
	XMLName      xml.Name       `xml:"DescribeInstancesResponse"`
	RequestId    string         `xml:"requestId"`
	Reservations []*Reservation `xml:"reservationSet>item"`
}

func (rsp *DescribeInstancesResponse) Instances() []*Instance {
	instances := []*Instance{}
	for _, r := range rsp.Reservations {
		for _, i := range r.Instances {
			instances = append(instances, i)
		}
	}
	return instances
}
