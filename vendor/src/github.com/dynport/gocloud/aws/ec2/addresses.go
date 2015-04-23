package ec2

import (
	"encoding/xml"
)

type Address struct {
	PublicIp                string `xml:"publicIp"`                // 203.0.113.41</publicIp>
	AllocationId            string `xml:"allocationId"`            // eipalloc-08229861</allocationId>
	Domain                  string `xml:"domain"`                  // vpc</domain>
	InstanceId              string `xml:"instanceId"`              // i-64600030</instanceId>
	AssociationId           string `xml:"associationId"`           // eipassoc-f0229899</associationId>
	NetworkInterfaceId      string `xml:"networkInterfaceId"`      // eni-ef229886</networkInterfaceId>
	NetworkInterfaceOwnerId string `xml:"networkInterfaceOwnerId"` // 053230519467</networkInterfaceOwnerId>
	PrivateIpAddress        string `xml:"privateIpAddress"`        // 10.0.0.228</privateIpAddress>
}

type DescribeAddressesResponse struct {
	Addresses []*Address `xml:"addressesSet>item"`
}

func (client *Client) DescribeAddresses() (addresses []*Address, e error) {
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), queryForAction("DescribeAddresses"), nil)
	if e != nil {
		return addresses, e
	}
	rsp := &DescribeAddressesResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return addresses, e
	}
	return rsp.Addresses, nil
}
