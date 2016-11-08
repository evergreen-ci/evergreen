package cloudformation

type RecordSet struct {
	AliasTarget     interface{}   `json:"AliasTarget,omitempty"`     // AliasTarget,
	Comment         interface{}   `json:"Comment,omitempty"`         // String,
	HostedZoneId    interface{}   `json:"HostedZoneId,omitempty"`    // String,
	HostedZoneName  interface{}   `json:"HostedZoneName,omitempty"`  // String,
	Name            interface{}   `json:"Name,omitempty"`            // String,
	Region          interface{}   `json:"Region,omitempty"`          // String,
	ResourceRecords []interface{} `json:"ResourceRecords,omitempty"` // [ String ],
	SetIdentifier   interface{}   `json:"SetIdentifier,omitempty"`   // String,
	TTL             interface{}   `json:"TTL,omitempty"`             // String,
	Type            interface{}   `json:"Type,omitempty"`            // String,
	Weight          interface{}   `json:"Weight,omitempty"`          // Integer
}
