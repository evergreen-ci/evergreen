package cloudformation

// http://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-properties-ec2-instance.html
type Ec2Instance struct {
	ImageId               interface{}           `json:"ImageId,omitempty"`
	DisableApiTermination interface{}           `json:"DisableApiTermination,omitempty"`
	KeyName               interface{}           `json:"KeyName,omitempty"`
	InstanceType          interface{}           `json:"InstanceType,omitempty"`
	SubnetId              interface{}           `json:"SubnetId,omitempty"`
	PrivateIpAddress      interface{}           `json:"PrivateIpAddress,omitempty"`
	NetworkInterfaces     []*NetworkInterface   `json:"NetworkInterfaces,omitempty"`
	UserData              string                `json:"UserData,omitempty"`
	BlockDeviceMappings   []*BlockDeviceMapping `json:"BlockDeviceMappings,omitempty"`
	Tags                  []*Tag                `json:"Tags,omitempty"`
}

type Ec2EIP struct {
	InstanceId interface{} `json:"InstanceId,omitempty"`
	Domain     interface{} `json:"Domain,omitempty"`
}
