package dynamodb

import "reflect"

type UpdateItem struct {
	AttributeUpdates            map[string]*AttributeUpdate `json:"AttributeUpdate"`
	Expected                    *Expected                   `json:"Expected"`
	Key                         *Item                       `json:"Key"`
	ReturnConsumedCapacity      string                      `json:"ReturnConsumedCapacity"`      // "string",
	ReturnItemCollectionMetrics string                      `json:"ReturnItemCollectionMetrics"` // "string",
	ReturnValues                string                      `json:"ReturnValues"`                // "string",
	TableName                   string                      `json:"TableName"`                   // "string"
}

type AttributeUpdate struct {
	Action string          `json:"Action"`
	Value  *ItemDefinition `json:"ItemDefinition,omitempty"`
}

func init() {
	ad := &AttributeDefinition{}
	reflect.ValueOf(ad).FieldByName("Action").Set(reflect.ValueOf("PUT"))
}
