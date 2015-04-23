package main

import (
	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocloud/cli/aws/ec2"
	"log"
	"os"
)

func main() {
	router := cli.NewRouter()
	router.Register("prices", &ec2.Prices{Region: os.Getenv("AWS_DEFAULT_REGION")}, "List ec2 prices")
	e := router.Run(append([]string{"prices"}, os.Args[1:]...)...)
	if e != nil {
		log.Fatal(e.Error())
	}
}

func init() {
	log.SetFlags(0)
}
