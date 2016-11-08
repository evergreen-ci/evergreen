package profitbricks

import (
	"encoding/xml"
)

const (
	SOAP_HEADER = `<?xml version="1.0" encoding="UTF-8"?><env:Envelope xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:tns="http://ws.api.profitbricks.com/" xmlns:env="http://schemas.xmlsoap.org/soap/envelope/"><env:Body>`
	SOAP_FOOTER = `</env:Body></env:Envelope>`
)

type Envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    *Body    `xml:"Body"`
}

type Body struct {
	XMLName                 xml.Name               `xml:"Body"`
	GetAllDataCenters       []*DataCenter          `xml:"getAllDataCentersResponse>return"`
	GetDataCenterResponse   *GetDataCenterResponse `xml:"getDataCenterResponse>return"`
	GetAllImagesResponse    []*Image               `xml:"getAllImagesResponse>return"`
	GetAllSnapshotsResponse []*Snapshot            `xml:"getAllSnapshotsResponse>return"`
	GetAllStoragesResponse  []*Storage             `xml:"getAllStoragesResponse>return"`
	GetAllServersResponse   []*Server              `xml:"getAllServersResponse>return"`
}

func NewSoapRequest(body string) string {
	return SOAP_HEADER + body + SOAP_FOOTER
}
