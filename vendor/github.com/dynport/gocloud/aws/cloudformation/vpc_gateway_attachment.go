package cloudformation

type VPCGatewayAttachment struct {
	InternetGatewayId interface{} `json:"InternetGatewayId,omitempty"`
	VpcId             interface{} `json:"VpcId,omitempty"`
	VpnGatewayId      string      `json:"VpnGatewayId,omitempty"`
}
