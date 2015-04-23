package main

import (
	"fmt"

	"github.com/dynport/gocli"
)

type keysList struct {
}

func (r *keysList) Run() error {
	cl, e := client()
	if e != nil {
		return e
	}
	rsp, e := cl.Keys()
	if e != nil {
		return e
	}
	t := gocli.NewTable()
	for _, k := range rsp.SshKeys {
		t.Add(k.Id, k.Name, fmt.Sprintf("%.64q...", k.PublicKey))
	}
	fmt.Println(t)

	return nil
}
