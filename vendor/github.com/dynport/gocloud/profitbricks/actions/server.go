package actions

import (
	"fmt"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/profitbricks"
	"strings"
)

func ListAllServersHandler() error {
	client := profitbricks.NewFromEnv()
	servers, e := client.GetAllServers()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Id", "Name", "ProvisioningState", "VmState", "Ips", "Lans")
	for _, server := range servers {
		table.Add(server.ServerId, server.ServerName, server.ProvisioningState, server.VirtualMachineState, strings.Join(server.Ips, ","), server.Lans())
	}
	fmt.Println(table)
	return nil
}
