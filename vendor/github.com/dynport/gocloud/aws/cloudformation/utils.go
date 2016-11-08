package cloudformation

type Reference struct {
	Ref string `json:"Ref,omitempty"`
}

func Ref(name string) *Reference {
	return &Reference{Ref: name}
}

func NewSubnet(az string, cidr string) *Resource {
	return NewResource("AWS::EC2::Subnet",
		&Subnet{
			AvailabilityZone: az,
			CidrBlock:        cidr,
			VpcId:            &Reference{Ref: "vpc"},
		},
	)
}

type Subnet struct {
	AvailabilityZone string      `json:"AvailabilityZone,omitempty"`
	CidrBlock        string      `json:"CidrBlock,omitempty"`
	Tags             []*Tag      `json:"Tags,omitempty"`
	VpcId            interface{} `json:"VpcId,omitempty"`
}
