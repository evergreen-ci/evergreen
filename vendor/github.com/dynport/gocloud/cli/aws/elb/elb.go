package elb

import (
	"fmt"
	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/aws/elb"
	"log"
	"strings"
)

func Register(router *cli.Router) {
	router.RegisterFunc("aws/elb/lbs/list", elbListLoadBalancers, "Describe load balancers")
	router.Register("aws/elb/lbs/describe", &elbDescribeLoadBalancer{}, "Describe load balancers")
	router.Register("aws/elb/lbs/deregister", &elbDeregisterInstances{}, "Deregister instances with load balancer")
	router.Register("aws/elb/lbs/register", &elbRegisterInstances{}, "Register instances with load balancer")
}

type elbDescribeLoadBalancer struct {
	Name string `cli:"type=arg required=true"`
}

func (a *elbDescribeLoadBalancer) Run() error {
	elbClient := elb.NewFromEnv()
	states, e := elbClient.DescribeInstanceHealth(a.Name)
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	for _, state := range states {
		stateString := ""
		if state.State == "InService" {
			stateString = gocli.Green(state.State)
		} else {
			stateString = gocli.Red(state.State)
		}
		table.Add(state.InstanceId, stateString)
	}
	fmt.Println(table)
	return nil
}

type elbRegisterInstances struct {
	LbId        string   `cli:"type=arg required=true"`
	InstanceIds []string `cli:"type=arg required=true"`
}

func (a *elbRegisterInstances) Run() error {
	return elb.NewFromEnv().RegisterInstancesWithLoadBalancer(a.LbId, a.InstanceIds)
}

type elbDeregisterInstances struct {
	LbId        string   `cli:"type=arg required=true"`
	InstanceIds []string `cli:"type=arg required=true"`
}

func (a *elbDeregisterInstances) Run() error {
	return elb.NewFromEnv().DeregisterInstancesWithLoadBalancer(a.LbId, a.InstanceIds)
}

func elbListLoadBalancers() error {
	elbClient := elb.NewFromEnv()
	log.Print("describing load balancers")
	lbs, e := elbClient.DescribeLoadBalancers()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Name", "DnsName", "Instances")
	for _, lb := range lbs {
		table.Add(
			lb.LoadBalancerName,
			lb.DNSName,
			strings.Join(lb.Instances, ", "),
		)
	}
	fmt.Print(table)
	return nil
}
