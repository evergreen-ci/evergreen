package ec2

type CreateNetworkInterface struct {
	DeviceIndex              int      `json:",omitempty"`
	AssociatePublicIpAddress bool     `json:",omitempty"`
	SubnetId                 string   `json:",omitempty"`
	SecurityGroupIds         []string `json:",omitempty"`
}
