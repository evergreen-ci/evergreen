package main

import (
	"fmt"

	"github.com/dynport/gocli"
)

type regionsList struct {
}

func (r *regionsList) Run() error {
	cl, e := client()
	if e != nil {
		return e
	}
	rsp, e := cl.Regions()
	if e != nil {
		return e
	}
	t := gocli.NewTable()
	for _, r := range rsp.Regions {
		t.Add(r.Slug, r.Name, r.Available)
	}
	fmt.Println(t)
	return nil
}
