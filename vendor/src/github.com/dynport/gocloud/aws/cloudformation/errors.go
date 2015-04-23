package cloudformation

import "encoding/xml"

type ErrorResponse struct {
	XMLName   xml.Name `xml:"ErrorResponse"`
	Error     *Error   `xml:"Error"`
	RequestId string   `xml:"RequestId"`
}

type Error struct {
	Type    string `xml:"Type,omitempty"`
	Code    string `xml:"Code,omitempty"`
	Message string `xml:"Message,omitempty"`
}
