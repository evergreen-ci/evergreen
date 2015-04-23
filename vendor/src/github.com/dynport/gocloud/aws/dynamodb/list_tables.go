package dynamodb

import "github.com/dynport/gocloud/aws"

type ListTables struct {
	ExclusiveStartTableName string `json:"ExclusiveStartTableName,omitempty"`
	Limit                   int    `json:"Limit,omitempty"`
}

func (l *ListTables) Execute(client *aws.Client) (*ListTablesResponse, error) {
	rsp := &ListTablesResponse{}
	e := loadAction(client, "ListTables", l, rsp)
	return rsp, e
}

type ListTablesResponse struct {
	LastEvaluatedTableName string   `json:"LastEvaluatedTableName,omitempty"`
	TableNames             []string `json:"TableNames,omitempty"`
}
