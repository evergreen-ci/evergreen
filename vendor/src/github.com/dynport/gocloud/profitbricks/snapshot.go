package profitbricks

import (
	"encoding/xml"
)

type Snapshot struct {
	SnapshotId        string `xml:"snapshotId"`        // type="xs:string"/>
	Description       string `xml:"description"`       // type="xs:string" minOccurs="0"/>
	SnapshotSize      int    `xml:"snapshotSize"`      // type="xs:long"/>
	SnapshotName      string `xml:"snapshotName"`      // type="xs:string" minOccurs="0"/>
	ProvisioningState string `xml:"provisioningState"` // type="tns:provisioningState"/>
	Bootable          bool   `xml:"bootable"`          // type="xs:boolean" minOccurs="0"/>
	OsType            string `xml:"osType"`            // type="tns:osType" minOccurs="0"/>
	CpuHotPlug        bool   `xml:"cpuHotPlug"`        // type="xs:boolean" minOccurs="0"/>
	RamHotPlug        bool   `xml:"ramHotPlug"`        // type="xs:boolean" minOccurs="0"/>
	NicHotPlug        bool   `xml:"nicHotPlug"`        // type="xs:boolean" minOccurs="0"/>
	NicHotUnPlug      bool   `xml:"nicHotUnPlug"`      // type="xs:boolean" minOccurs="0"/>
	Region            string `xml:"region"`            // type="tns:region"/>
}

type UpdateSnapshotRequest struct {
	XMLName      xml.Name `xml:"request"`
	SnapshotId   string   `xml:"snapshotId"`   // type="xs:string"/>
	Description  string   `xml:"description"`  // type="xs:string" minOccurs="0"/>
	SnapshotName string   `xml:"snapshotName"` // type="xs:string" minOccurs="0"/>
	Bootable     bool     `xml:"bootable"`     // type="xs:boolean" minOccurs="0"/>
	OsType       string   `xml:"osType"`       // type="tns:osType" minOccurs="0"/>
	CpuHotPlug   string   `xml:"cpuHotPlug"`   // type="xs:boolean" minOccurs="0"/>
	RamHotPlug   string   `xml:"ramHotPlug"`   // type="xs:boolean" minOccurs="0"/>
	NicHotPlug   string   `xml:"nicHotPlug"`   // type="xs:boolean" minOccurs="0"/>
	NicHotUnPlug string   `xml:"nicHotUnPlug"` // type="xs:boolean" minOccurs="0"/>
}

type RollbackSnapshotRequest struct {
	XMLName    xml.Name `xml:"request"`
	SnapshotId string   `xml:"snapshotId"`
	StorageId  string   `xml:"storageId"`
}

func (client *Client) RollbackSnapshot(req *RollbackSnapshotRequest) error {
	_, e := client.loadSoapRequest(marshalMultiRequest("rollbackSnapshot", req))
	return e
}

func (client *Client) CreateSnapshot(req *UpdateSnapshotRequest) error {
	_, e := client.loadSoapRequest(marshalMultiRequest("createSnapshot", req))
	return e
}

func (client *Client) UpdateSnapshot(req *UpdateSnapshotRequest) error {
	_, e := client.loadSoapRequest(marshalMultiRequest("updateSnapshot", req))
	return e
}
