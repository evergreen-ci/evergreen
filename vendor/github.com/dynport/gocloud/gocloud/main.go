package main

import (
	"log"
	"os"

	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocloud/cli/aws/cloudformation"
	"github.com/dynport/gocloud/cli/aws/cloudwatch"
	"github.com/dynport/gocloud/cli/aws/ec2"
	"github.com/dynport/gocloud/cli/aws/elb"
	"github.com/dynport/gocloud/cli/aws/iam"
	"github.com/dynport/gocloud/cli/aws/route53"
	"github.com/dynport/gocloud/cli/digitalocean"
	"github.com/dynport/gocloud/cli/hetzner"
	"github.com/dynport/gocloud/cli/jiffybox"
	"github.com/dynport/gocloud/cli/profitbricks"
)

var router = cli.NewRouter()

func init() {
	ec2.Register(router)
	cloudformation.Register(router)
	elb.Register(router)
	digitalocean.Register(router)
	hetzner.Register(router)
	jiffybox.Register(router)
	route53.Register(router)
	iam.Register(router)
	cloudwatch.Register(router)
	profitbricks.Register(router)
}

func init() {
	log.SetFlags(0)
}

func main() {
	if e := router.RunWithArgs(); e != nil {
		if e != cli.ErrorNoRoute {
			log.Println("ERROR: " + e.Error())
		}
		os.Exit(1)
	}
}
