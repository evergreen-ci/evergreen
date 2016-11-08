package cloudformation

func NewSubnetRouteTableAssociation(routeTableId interface{}, subnetId interface{}) *Resource {
	return NewResource("AWS::EC2::SubnetRouteTableAssociation",
		&SubnetRouteTableAssociation{RouteTableId: routeTableId, SubnetId: subnetId},
	)
}

type SubnetRouteTableAssociation struct {
	RouteTableId interface{} `json:"RouteTableId,omitempty"`
	SubnetId     interface{} `json:"SubnetId,omitempty"`
}
