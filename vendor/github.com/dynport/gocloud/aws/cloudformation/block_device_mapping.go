package cloudformation

var DefaultBlockDeviceMapping = &BlockDeviceMapping{
	DeviceName: "/dev/sda1",
	Ebs: &Ebs{
		VolumeSize: "8",
	},
}
