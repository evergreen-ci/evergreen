package cloudformation

type NetworkInterface struct {
	AssociatePublicIpAddress       bool          `json:"AssociatePublicIpAddress,omitempty"`       // : Boolean,
	DeleteOnTermination            bool          `json:"DeleteOnTermination,omitempty"`            // : Boolean,
	Description                    string        `json:"Description,omitempty"`                    // : String,
	DeviceIndex                    string        `json:"DeviceIndex,omitempty"`                    // : String,
	GroupSet                       []interface{} `json:"GroupSet,omitempty"`                       // : [ String, ... ],
	NetworkInterfaceId             string        `json:"NetworkInterfaceId,omitempty"`             // : String,
	PrivateIpAddress               string        `json:"PrivateIpAddress,omitempty"`               // : String,
	PrivateIpAddresses             []string      `json:"PrivateIpAddresses,omitempty"`             // : [ PrivateIpAddressSpecification, ... ],
	SecondaryPrivateIpAddressCount int           `json:"SecondaryPrivateIpAddressCount,omitempty"` // : Integer,
	SubnetId                       interface{}   `json:"SubnetId,omitempty"`                       // : String
}
