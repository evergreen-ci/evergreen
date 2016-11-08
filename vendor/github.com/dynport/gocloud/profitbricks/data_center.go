package profitbricks

type DataCenter struct {
	DataCenterId      string     `xml:"dataCenterId"`
	DataCenterName    string     `xml:"dataCenterName"`
	DataCenterVersion int        `xml:"dataCenterVersion"`
	Servers           []*Server  `xml:"servers"`
	Storages          []*Storage `xml:"storages"`
	ProvisioningState string     `xml:"provisioningState"`
	Region            string     `xml:"region"`
}
