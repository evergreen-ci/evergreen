package profitbricks

type ConnectedStorage struct {
	BootDevice   bool   `xml:"bootDevice"`   // >true</bootDevice>
	BusType      string `xml:"busType"`      // >VIRTIO</busType>
	DeviceNumber int    `xml:"deviceNumber"` // >1</deviceNumber>
	Size         int    `xml:"size"`         // >10</size>
	StorageId    string `xml:"storageId"`    // >dd24d153-6cf7-4476-aab3-0b1d32d5e15c</storageId>
	StorageName  string `xml:"storageName"`  // >Ubuntu</storageName>
}
