package main

import (
	"log"
	"os"

	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocloud/digitalocean/v2/digitalocean"
)

var logger = log.New(os.Stderr, "", 0)

func main() {
	router := cli.NewRouter()
	router.Register("images/list", &imagesList{}, "List Images")
	router.Register("regions/list", &regionsList{}, "List Regions")
	router.Register("keys/list", &keysList{}, "List Keys")
	router.Register("droplets/delete", &dropletDelete{}, "Delete Droplet")
	router.Register("droplets/list", &dropletsList{}, "List Droplets")
	router.Register("droplets/show", &dropletShow{}, "Show Droplet")
	router.Register("droplets/create", &dropletCreate{}, "Create Droplet")
	switch e := router.RunWithArgs(); e {
	case nil, cli.ErrorHelpRequested, cli.ErrorNoRoute:
		// ignore
		return
	default:
		logger.Fatal(e)
	}
}

func client() (*digitalocean.Client, error) {
	return digitalocean.NewFromEnv()
}
