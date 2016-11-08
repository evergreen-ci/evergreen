package cloudformation

type Route struct {
	DestinationCidrBlock interface{} `json:"DestinationCidrBlock,omitempty"`
	GatewayId            interface{} `json:"GatewayId,omitempty"`
	RouteTableId         interface{} `json:"RouteTableId,omitempty"`
}
