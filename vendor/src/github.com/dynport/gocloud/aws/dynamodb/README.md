# DynamoDB

## Usage

	package main

	import (
		"log"
		"os"
		"strings"

		"github.com/dynport/gocloud/aws"
		"github.com/dynport/gocloud/aws/dynamodb"
	)

	var tableName = os.Getenv("TABLE_NAME")

	func run() error {
		client := aws.NewFromEnv()

		// list tables
		header("ListTables")
		listTabels := &dynamodb.ListTables{}
		if rsp, e := listTabels.Execute(client); e != nil {
			return e
		} else {
			log.Printf("tables: %v", rsp.TableNames)
		}

		// describe table
		header("DescribeTable")
		describeTable := &dynamodb.DescribeTable{TableName: tableName}
		if rsp, e := describeTable.Execute(client); e != nil {
			return e
		} else if rsp.Table != nil {
			log.Printf("name: %q", rsp.Table.TableName)
			log.Printf("status: %q", rsp.Table.TableStatus)
			log.Printf("item count: %d", rsp.Table.ItemCount)
			log.Printf("size in bytes: %d", rsp.Table.TableSizesBytes)
			if rsp.Table.ProvisionedThroughput != nil {
				log.Printf("read: %d", rsp.Table.ProvisionedThroughput.ReadCapacityUnits)
				log.Printf("write: %d", rsp.Table.ProvisionedThroughput.WriteCapacityUnits)
				log.Printf("decreases: %d", rsp.Table.ProvisionedThroughput.NumberOfDecreasesToday)
			}
		}

		// put item
		header("PutItem")
		put := &dynamodb.PutItem{
			TableName: tableName,
			Item: dynamodb.Item{
				"Key":   {S: "hello"},
				"Value": {S: "world"},
			},
			ReturnConsumedCapacity: "TOTAL",
		}

		if _, e := put.Execute(client); e != nil {
			return e
		} else {
			log.Printf("put item!")
		}

		// get item
		header("GetItem")
		item := &dynamodb.Item{"Key": {S: "hello"}}
		getItem := &dynamodb.GetItem{
			TableName: tableName,
			Key:       item,
			ReturnConsumedCapacity: "TOTAL",
			ConsistentRead:         "true",
		}

		if rsp, e := getItem.Execute(client); e != nil {
			return e
		} else {
			if rsp.ConsumedCapacity != nil {
				log.Printf("consumed capacity: %.1f", rsp.ConsumedCapacity.CapacityUnits)
			}
			for k, v := range rsp.Item {
				log.Printf("%s: %#v", k, v)

			}
		}

		// scan
		header("Scan")
		scan := &dynamodb.Scan{
			TableName: tableName,
			Limit:     10,
			ReturnConsumedCapacity: "TOTAL",
		}
		if sr, e := scan.Execute(client); e != nil {
			return e
		} else {
			log.Printf("Count: %d", sr.Count)
			if sr.ConsumedCapacity != nil {
				log.Printf("ConsumedCapacity: %.1f", sr.ConsumedCapacity.CapacityUnits)
			}
			for _, i := range sr.Items {
				for k, v := range i {
					log.Printf("%s: %#v", k, v)

				}
			}
		}
		return nil
	}

	func main() {
		log.SetFlags(0)
		e := run()
		if e != nil {
			log.Printf("ERROR: %q", e.Error())
			log.Fatal(e)
		}
	}

	func header(message string) {
		log.Printf("%s %s %s", strings.Repeat("*", 50), message, strings.Repeat("*", 50))
	}
