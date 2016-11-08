package actions

import (
	"fmt"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/profitbricks"
	"strings"
)

const (
	ENV_PROFITBRICKS_DEFAULT_DATA_CENTER_ID = "PROFITBRICKS_DEFAULT_DC_ID"
)

var DescribeDataCenter *gocli.Action

type DescribeDataCenterHandler struct {
	DataCenterId string `cli:"type=arg required=true"`
}

func (a *DescribeDataCenterHandler) Run() error {
	client := profitbricks.NewFromEnv()
	dc, e := client.GetDataCenter(a.DataCenterId)
	if e != nil {
		return e
	}

	table := gocli.NewTable()
	table.Add("Id", dc.DataCenterId)
	table.Add("Name", dc.DataCenterName)
	table.Add("Region", dc.Region)
	table.Add("State", dc.ProvisioningState)
	table.Add("Version", dc.DataCenterVersion)
	fmt.Println(table)
	fmt.Println("\nServers:")
	if len(dc.Servers) > 0 {
		table = gocli.NewTable()
		table.Add("Id", "Created", "Name", "Lans", "Ip", "AZ", "ProvState", "VMState", "Ram", "Cores", "Internet")
		for _, server := range dc.Servers {
			table.Add(server.ServerId, server.CreationTime.Format("2006-01-02T15:04"), server.ServerName, server.Lans(), strings.Join(server.Ips, ","), server.AvailabilityZone, server.ProvisioningState, server.VirtualMachineState, server.Ram, server.Cores, server.InternetAccess)
		}
		fmt.Println(table)
	} else {
		fmt.Println("* None *")
	}

	fmt.Println("\nStorages:")
	if len(dc.Storages) > 0 {
		table = gocli.NewTable()
		table.Add("Id", "Name", "Size")
		for _, storage := range dc.Storages {
			table.Add(storage.StorageId, storage.StorageName, storage.Size)
		}
		fmt.Println(table)
	} else {
		fmt.Println("* None *")
	}
	return nil
}
