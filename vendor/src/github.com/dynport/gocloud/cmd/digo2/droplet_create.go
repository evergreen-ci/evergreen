package main

import (
	"strings"

	"github.com/dynport/gocloud/digitalocean/v2/digitalocean"
)

type dropletCreate struct {
	Name              string `cli:"opt --name required"`   // required
	Region            string `cli:"opt --region required"` // required
	Size              string `cli:"opt --size required"`   // required
	Image             string `cli:"opt --image required"`  // required
	SshKeys           string `cli:"opt --ssh-keys"`
	Backups           bool   `cli:"opt --backups"`
	IPv6              bool   `cli:"opt --ipv6"`
	PrivateNetworking bool   `cli:"opt --private-networking"`
}

func (r *dropletCreate) Run() error {
	cl, e := client()
	if e != nil {
		return e
	}
	a := digitalocean.CreateDroplet{
		Name:              r.Name,
		Region:            r.Region,
		Size:              r.Size,
		Image:             r.Image,
		Backups:           r.Backups,
		IPv6:              r.IPv6,
		PrivateNetworking: r.PrivateNetworking,
	}
	if r.SshKeys != "" {
		a.SshKeys = strings.Split(r.SshKeys, ",")
	}

	rsp, e := a.Execute(cl)
	if e != nil {
		return e
	}
	printDroplet(rsp.Droplet)
	return nil
}
