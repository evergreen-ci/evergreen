package dynamodb

import "github.com/dynport/gocloud/aws"

type PutItem struct {
	TableName                   string               `json:"TableName,omitempty"`
	Expected                    map[string]*Expected `json:"Expected,omitempty"`
	Item                        Item                 `json:"Item,omitempty"`
	ReturnValues                string               `json:"ReturnValues,omitempty"`
	ReturnConsumedCapacity      string               `json:"ReturnConsumedCapacity,omitempty"`
	ReturnItemCollectionMetrics string               `json:"ReturnItemCollectionMetrics,omitempty"`
}

type Expected struct {
	Exists string          `json:"Exists,omitempty"`
	Value  *ItemDefinition `json:"Value,omitempty"`
}

func (p *PutItem) Execute(client *aws.Client) (*PutItemResponse, error) {
	rsp := &PutItemResponse{}
	e := loadAction(client, "PutItem", p, rsp)
	return rsp, e
}

type PutItemResponse struct{}

type Item map[string]*ItemDefinition

type ItemDefinition struct {
	S  string   `json:"S,omitempty"`
	SS []string `json:"SS,omitempty"`
	B  string   `json:"B,omitempty"`
	BS []string `json:"BS,omitempty"`
	N  string   `json:"N,omitempty"`
	NS []string `json:"NS,omitempty"`
}
