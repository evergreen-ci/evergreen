package ec2

import (
	"encoding/xml"
	"net/url"
	"strconv"
)

type DescribeSecurityGroupsResponse struct {
	SecurityGroups []*SecurityGroup `xml:"securityGroupInfo>item"`
}

type IpPermission struct {
	IpProtocol string           `xml:"ipProtocol,omitempty"`  // tcp</ipProtocol>
	FromPort   int              `xml:"fromPort,omitempty"`    // 80</fromPort>
	ToPort     int              `xml:"toPort,omitempty"`      // 80</toPort>
	Groups     []*SecurityGroup `xml:"groups>item,omitempty"` //
	IpRanges   []string         `xml:"ipRanges>item>cidrIp"`
}

type SecurityGroup struct {
	OwnerId          string          `xml:"ownerId,omitempty"`          // 111122223333</ownerId>
	GroupId          string          `xml:"groupId,omitempty"`          // sg-1a2b3c4d</groupId>
	GroupName        string          `xml:"groupName,omitempty"`        // WebServers</groupName>
	GroupDescription string          `xml:"groupDescription,omitempty"` // Web Servers</groupDescription>
	VpcId            string          `xml:"vpcId,omitempty"`            //
	IpPermissions    []*IpPermission `xml:"ipPermissions>item"`
}

type DescribeSecurityGroupsParameters struct {
	GroupNames []string
	GroupIds   []string
	Filters    []*Filter
}

func (d *DescribeSecurityGroupsParameters) query() string {
	v := url.Values{}
	for i, g := range d.GroupIds {
		v.Add("GroupId."+strconv.Itoa(i+1), g)
	}
	for i, g := range d.GroupNames {
		v.Add("GroupName."+strconv.Itoa(i+1), g)
	}
	if len(v) > 0 {
		return v.Encode()
	}
	return ""
}

func (client *Client) DescribeSecurityGroups(params *DescribeSecurityGroupsParameters) (groups []*SecurityGroup, e error) {
	if params == nil {
		params = &DescribeSecurityGroupsParameters{}
	}
	q := queryForAction("DescribeSecurityGroups")
	if search := params.query(); search != "" {
		q += "&" + search
	}
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), q, nil)
	if e != nil {
		return groups, e
	}
	rsp := &DescribeSecurityGroupsResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return groups, e
	}
	return rsp.SecurityGroups, nil
}
