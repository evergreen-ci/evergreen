package cloudformation

type RouteTable struct {
	VpcId interface{} `json:"VpcId,omitempty"`
	Tags  []*Tag      `json:"Tags,omitempty"`
}
