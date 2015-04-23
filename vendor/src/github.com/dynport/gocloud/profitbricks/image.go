package profitbricks

type Image struct {
	CpuHotpluggable    bool   `xml:"cpuHotpluggable"`    // type="xs:boolean" minOccurs="0"/>
	ImageId            string `xml:"imageId"`            // type="xs:string" minOccurs="0"/>
	ImageName          string `xml:"imageName"`          // type="xs:string" minOccurs="0"/>
	ImageSize          int    `xml:"imageSize"`          // type="xs:long" minOccurs="0"/>
	ImageType          string `xml:"imageType"`          // type="tns:imageType" minOccurs="0"/>
	MemoryHotpluggable bool   `xml:"memoryHotpluggable"` // type="xs:boolean" minOccurs="0"/>
	OsType             string `xml:"osType"`             // type="tns:osType" minOccurs="0"/>
	Region             string `xml:"region"`             // type="tns:region" minOccurs="0"/>
	ServerIds          string `xml:"serverIds"`          // type="xs:string" nillable="true" minOccurs="0" maxOccurs="unbounded"/>
	Writeable          bool   `xml:"writeable"`          // type="xs:boolean" minOccurs="0"/>
}
