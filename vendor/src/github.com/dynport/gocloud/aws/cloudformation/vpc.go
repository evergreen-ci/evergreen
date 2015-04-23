package cloudformation

type Vpc struct {
	CidrBlock          string `json:"CidrBlock,omitempty"`
	EnableDnsSupport   bool   `json:"EnableDnsSupport,omitempty"`
	EnableDnsHostnames bool   `json:"EnableDnsHostnames,omitempty"`
	InstanceTenancy    string `json:"InstanceTenancy,omitempty"`
	Tags               []*Tag `json:"Tags,omitempty"`
}
