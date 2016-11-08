package main

import (
	"fmt"

	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/digitalocean/v2/digitalocean"
)

type dropletShow struct {
	Id string `cli:"arg required"`
}

func (r *dropletShow) Run() error {
	cl, e := client()
	if e != nil {
		return e
	}
	rsp, e := cl.Droplet(r.Id)
	if e != nil {
		return e
	}
	printDroplet(rsp.Droplet)
	return nil
}

func printDroplet(droplet *digitalocean.Droplet) {
	t := gocli.NewTable()
	t.Add("Id", droplet.Id)
	t.Add("Name", droplet.Name)
	t.Add("Status", droplet.Status)
	t.Add("Locked", fmt.Sprintf("%t", droplet.Locked))
	t.Add("CreatedAt", droplet.CreatedAt.Format("2006-01-02 15:04:05"))
	t.Add("Size", droplet.Size.Slug)
	t.Add("Region", droplet.Region.Name)
	t.Add("Image", droplet.Image.Name)
	for i, ip := range droplet.Networks.V4 {
		t.Add(fmt.Sprintf("IP %d", i+1), ip.IpAddress, ip.Type)
	}
	fmt.Println(t)
}
