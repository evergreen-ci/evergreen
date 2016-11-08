package dynamodb

import "github.com/dynport/gocloud/aws"

type DescribeTable struct {
	TableName string
}

func (d *DescribeTable) Execute(client *aws.Client) (*DescribeTableResponse, error) {
	rsp := &DescribeTableResponse{}
	e := loadAction(client, "DescribeTable", d, rsp)
	return rsp, e
}

type DescribeTableResponse struct {
	Table *Table `json:"Table,omitempty"`
}

type Table struct {
	TableStatus           string                 `json:"TableStatus,omitempty"`
	TableSizesBytes       int                    `json:"TableSizesBytes,omitempty"`
	TableName             string                 `json:"TableName,omitempty"`
	ProvisionedThroughput *ProvisionedThroughput `json:"ProvisionedThroughput,omitempty"`
	KeySchema             []*KeySchema           `json:"KeySchema"`
	ItemCount             int                    `json:"ItemCount,omitempty"`
	CreationDateTime      float64                `json:"CreationDateTime,omitempty"`
	AttributeDefinitions  []*AttributeDefinition `json:"AttributeDefinitions,omitempty"`
}

type AttributeDefinition struct {
	AttributeType string
	AtributeName  string
}

type ProvisionedThroughput struct {
	WriteCapacityUnits     int
	ReadCapacityUnits      int
	NumberOfDecreasesToday int
}

type KeySchema struct {
	KeyType       string
	AttributeName string
}
