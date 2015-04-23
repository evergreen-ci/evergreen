package dynamodb

import "github.com/dynport/gocloud/aws"

type Scan struct {
	TableName              string                 `json:"TableName,omitempty"`
	TotalSegments          string                 `json:"TotalSegments,omitempty"`
	AttributesToGet        []string               `json:"AttributesToGet,omitempty"`
	ExclusiveStartKey      Item                   `json:"ExclusiveStartKey,omitempty"`
	Limit                  int                    `json:"Limit,omitempty"`
	ReturnConsumedCapacity string                 `json:"ReturnConsumedCapacity,omitempty"` // INDEXES, TOTAL or NONE
	ScanFilter             map[string]*ScanFilter `json:"ScanFilter,omitempty"`
	Segment                string                 `json:"Segment,omitempty"`
	Select                 string                 `json:"Select,omitempty"`
}

func (s *Scan) Execute(client *aws.Client) (*ScanResponse, error) {
	rsp := &ScanResponse{}
	e := loadAction(client, "Scan", s, rsp)
	return rsp, e
}

type ScanResponse struct {
	ConsumedCapacity *ConsumedCapacity
	Count            int    `json:"Count,omitempty"`
	ScannedCount     int    `json:"ScannedCount,omitempty"`
	Items            []Item `json:"Items,omitempty"`
	LastEvaluatedKey Item   `json:"tEvaluatedKey,omitempty"`
}

type ScanFilter struct {
	AttributeValueList []*ItemDefinition `json:"AttributeValueList,omitempty"`
	ComparisonOperator string            `json:"parisonOperator,omitempty"`
}
