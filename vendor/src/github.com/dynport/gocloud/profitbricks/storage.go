package profitbricks

import (
	"encoding/xml"
	"time"
)

type Storage struct {
	DataCenterId         string    `xml:"dataCenterId,omitempty"`
	DataCenterVersion    string    `xml:"dataCenterVersion,omitempty"`
	StorageId            string    `xml:"storageId"`
	Size                 int       `xml:"size"`
	StorageName          string    `xml:"storageName"`
	BootDevice           bool      `xml:"bootDevice"`
	BusType              string    `xml:"busType"`
	DeviceNumber         int       `xml:"deviceNumber"`
	ImageId              string    `xml:"mountImage>imageId,omitempty"`
	ImageName            string    `xml:"mountImage>imageName,omitempty"`
	ServerIds            []string  `xml:"serverIds,omitempty"`
	ProvisioningState    string    `xml:"provisioningState,omitempty"`
	CreationTime         time.Time `xml:"creationTime,omitempty"`
	LastModificationTime time.Time `xml:"lastModificationTime,omitempty"`
}

type CreateStorageRequest struct {
	XMLName                   xml.Name `xml:"request"`
	DataCenterId              string   `xml:"dataCenterId,omitempty"`
	StorageName               string   `xml:"storageName,omitempty"`
	Size                      int      `xml:"size,omitempty"`
	MountImageId              string   `xml:"mountImageId,omitempty"`
	ProfitBricksImagePassword string   `xml:"profitBricksImagePassword,omitempty"`
}
