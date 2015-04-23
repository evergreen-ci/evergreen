package main

import (
	"fmt"
	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocloud/aws"
	"github.com/dynport/gocloud/aws/route53"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type Action struct {
	HostedZoneId string `cli:"type=arg required=true"`
	Domain       string `cli:"type=arg required=true"`
}

func mustGetEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("%s must be set", key)
	}
	return value, nil
}

func clientFromEnv() (*route53.Client, error) {
	errs := []string{}
	awsClient := &aws.Client{}
	var e error
	awsClient.Key, e = mustGetEnv("AWS_ACCESS_KEY_ID")
	if e != nil {
		errs = append(errs, e.Error())
	}
	awsClient.Secret, e = mustGetEnv("AWS_SECRET_ACCESS_KEY")
	if e != nil {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf(strings.Join(errs, ", "))
	}
	return &route53.Client{Client: awsClient}, nil
}

func (action *Action) Run() error {
	client, e := clientFromEnv()
	if e != nil {
		return e
	}

	sets, e := client.ListResourceRecordSets(action.HostedZoneId)
	if e != nil {
		return e
	}
	name := action.Domain
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	var deleteRecord *route53.ResourceRecordSet

	for _, s := range sets {
		if s.Name == name {
			deleteRecord = s
			break
		}
	}
	ip, e := currentIp()
	if e != nil {
		return e
	}
	log.Printf("current ip is %s", ip)
	changes := []*route53.Change{}
	if deleteRecord != nil {
		for _, value := range deleteRecord.ResourceRecords {
			if value.Value == ip {
				log.Printf("nothing changed (ip is %s)", ip)
				return nil
			}
		}
		log.Print("deleting record before creating")
		changes = append(changes, &route53.Change{
			Action:            "DELETE",
			ResourceRecordSet: deleteRecord,
		},
		)
	} else {
		log.Print("no previous record exists => only creating")
	}
	changes = append(changes, &route53.Change{
		Action: "CREATE",
		ResourceRecordSet: &route53.ResourceRecordSet{
			Name:            name,
			Type:            "A",
			TTL:             60,
			ResourceRecords: []*route53.ResourceRecord{{Value: ip}},
		},
	},
	)
	e = client.ChangeResourceRecordSets(action.HostedZoneId, changes)
	if e != nil {
		return e
	}
	log.Printf("created entry for %s", action.Domain)
	return nil
}

var currentIpRegexp = regexp.MustCompile("Current IP Address: ([\\d]+\\.[\\d]+\\.[\\d]+\\.[\\d]+)")

func currentIp() (string, error) {
	rsp, e := http.Get("http://checkip.dyndns.com")
	if e != nil {
		return "", e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return "", e
	}
	match := currentIpRegexp.FindStringSubmatch(string(b))
	if len(match) > 1 {
		return match[1], nil
	}
	return "", fmt.Errorf("unable to extract IP from %s", string(b))
}

func main() {
	log.SetFlags(0)
	e := cli.RunActionWithArgs(&Action{})
	switch e {
	case nil, cli.ErrorHelpRequested, cli.ErrorNoRoute:
		//
	default:
		log.Printf("ERROR: %s", e.Error())
	}
}
