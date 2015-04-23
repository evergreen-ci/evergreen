package main

import (
	"fmt"

	"github.com/dynport/gocli"
)

type dropletsList struct {
}

func (r *dropletsList) Run() error {
	cl, e := client()
	if e != nil {
		return e
	}
	rsp, e := cl.Droplets()
	if e != nil {
		return e
	}
	t := gocli.NewTable()
	t.Add("Id", "Status", "Ip", "Name", "Region", "Size", "ImageId:ImageName (ImageSlug)", "CreatedAt")
	for _, d := range rsp.Droplets {
		imageName := fmt.Sprintf("%d:%s", d.Image.Id, d.Image.Name)
		if d.Image.Slug != "" {
			imageName += " (" + d.Image.Slug + ")"
		}
		ip := ""
		if len(d.Networks.V4) > 0 {
			ip = d.Networks.V4[0].IpAddress
		}
		t.Add(d.Id, d.Status, ip, d.Name, d.Region.Slug, d.Size.Slug, imageName, d.CreatedAt.Format("2006-01-02T15:04:05"))
	}
	fmt.Println(t)
	return nil
}
