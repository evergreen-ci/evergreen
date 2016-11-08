package hetzner

import (
	"fmt"
	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/hetzner"
	"github.com/dynport/gologger"
)

var logger = gologger.NewFromEnv()

func Register(router *cli.Router) {
	router.RegisterFunc("hetzner/servers/list", ListServers, "list servers")
	router.Register("hetzner/servers/describe", &DescribeServer{}, "describe server")
	router.Register("hetzner/servers/rename", &RenameServer{}, "rename server")
}

type DescribeServer struct {
	IP string `cli:"type=arg required=true"`
}

func (a *DescribeServer) Run() error {
	account, e := hetzner.AccountFromEnv()
	if e != nil {
		return e
	}

	server, e := account.LoadServer(a.IP)
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("IP", server.ServerIp)
	table.Add("Number", server.ServerNumber)
	table.Add("Name", server.ServerName)
	table.Add("Product", server.Product)
	table.Add("DataCenter", server.Dc)
	table.Add("Status", server.Status)
	table.Add("Reset", server.Reset)
	table.Add("Rescue", server.Rescue)
	table.Add("VNC", server.Vnc)
	fmt.Println(table)
	return nil
}

func ListServers() error {
	account, e := hetzner.AccountFromEnv()
	if e != nil {
		return e
	}
	servers, e := account.Servers()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Number", "Name", "Product", "DC", "Ip", "Status")
	for _, server := range servers {
		table.Add(server.ServerNumber, server.ServerName, server.Product, server.Dc, server.ServerIp, server.Status)
	}
	fmt.Println(table)
	return nil
}

type RenameServer struct {
	Ip      string `cli:"type=arg required=true"`
	NewName string `cli:"type=arg required=true"`
}

func (a *RenameServer) Run() error {
	account, e := hetzner.AccountFromEnv()
	if e != nil {
		return e
	}
	logger.Infof("renaming servers %s to %s", a.Ip, a.NewName)
	return account.RenameServer(a.Ip, a.NewName)
}
