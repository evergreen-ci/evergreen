package profitbricks

type Nic struct {
	DataCenterId      string    `xml:"dataCenterId"`      // eb394325-b2f1-418c-93e4-377697e3c597</dataCenterId>
	DataCenterVersion int       `xml:"dataCenterVersion"` // 4</dataCenterVersion>
	NicId             string    `xml:"nicId"`             // 910910a2-a4c5-45fe-aeff-fe7ec4d8d4d9</nicId>
	LanId             int       `xml:"lanId"`             // 1</lanId>
	InternetAccess    bool      `xml:"internetAccess"`    // true</internetAccess>
	ServerId          string    `xml:"serverId"`          // 5d0a9936-94c0-44a8-85cc-9a9cae082202</serverId>
	Ips               string    `xml:"ips"`               // 46.16.78.173</ips>
	MacAddress        string    `xml:"macAddress"`        // 02:01:0f:f6:2c:b5</macAddress>
	Firewall          *Firewall `xml:"firewall"`
	DhcpActive        bool      `xml:"dhcpActive"`        // >true</dhcpActive>
	GatewayIp         string    `xml:"gatewayIp"`         // >46.16.78.1</gatewayIp>
	ProvisioningState string    `xml:"provisioningState"` // >AVAILABLE</provisioningState>
}
