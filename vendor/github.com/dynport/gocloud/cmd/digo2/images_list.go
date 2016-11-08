package main

import (
	"fmt"
	"strings"

	"github.com/dynport/gocli"
)

type imagesList struct {
	Page int `cli:"opt --page"`
}

func (r *imagesList) Run() error {
	cl, e := client()
	if e != nil {
		return e
	}
	rsp, e := cl.Images(r.Page)
	if e != nil {
		return e
	}
	t := gocli.NewTable()
	for _, i := range rsp.Images {
		t.Add(i.Id, i.Slug, i.Name, strings.Join(i.Regions, ","))
	}
	fmt.Println(t)
	return nil
}
