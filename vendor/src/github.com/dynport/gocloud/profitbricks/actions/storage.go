package actions

import (
	"fmt"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/profitbricks"
	"strings"
)

func ListAllStorages() error {
	client := profitbricks.NewFromEnv()
	storages, e := client.GetAllStorages()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Id", "Name", "ProvisioningState", "Servers", "Image Name", "Image ID")
	for _, storage := range storages {
		table.Add(storage.StorageId, storage.StorageName, storage.ProvisioningState, strings.Join(storage.ServerIds, ","), storage.ImageName, storage.ImageId)
	}
	fmt.Println(table)
	return nil
}
