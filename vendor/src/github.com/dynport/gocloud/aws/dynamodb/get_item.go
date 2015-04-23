package dynamodb

import "github.com/dynport/gocloud/aws"

type GetItem struct {
	AttributesToGet        []string `json:"AttributesToGet,omitempty"`
	ConsistentRead         string   `json:"ConsistentRead,omitempty"`
	Key                    *Item    `json:"Key,omitempty"`
	ReturnConsumedCapacity string   `json:"ReturnConsumedCapacity,omitempty"`
	TableName              string   `json:"TableName,omitempty"`
}

func (g *GetItem) Execute(client *aws.Client) (*GetItemResponse, error) {
	rsp := &GetItemResponse{}
	e := loadAction(client, "GetItem", g, rsp)
	return rsp, e
}

type GetItemResponse struct {
	ConsumedCapacity *ConsumedCapacity `json:"ConsumedCapacity,omitempty"`
	Item             Item              `json:"Item,omitempty"`
}

type ConsumedCapacity struct {
	CapacityUnits float64 `json:"CapacityUnits,omitempty"`
	TableName     string  `json:"TableName,omitempty"`
}
