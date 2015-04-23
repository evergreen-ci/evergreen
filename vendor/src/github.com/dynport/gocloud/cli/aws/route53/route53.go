package route53

import (
	"fmt"
	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/aws/route53"
	"log"
)

func Register(router *cli.Router) {
	router.RegisterFunc("aws/route53/hosted-zones/list", route53ListHostedZones, "List Hosted Zones")
	router.Register("aws/route53/rrs/list", &route53ListResourceRecordSet{}, "List Resource Record Set")
}

func route53ListHostedZones() error {
	log.Print("describing hosted zones")
	client := route53.NewFromEnv()
	zones, e := client.ListHostedZones()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Name", "record_set_count", "rrs count")
	for _, zone := range zones {
		table.Add(zone.Code(), zone.Name, zone.ResourceRecordSetCount)
	}
	fmt.Println(table)
	return nil
}

type route53ListResourceRecordSet struct {
	HostedZone string `cli:"type=arg required=true"`
}

func (r *route53ListResourceRecordSet) Run() error {
	client := route53.NewFromEnv()
	rrsets, e := client.ListResourceRecordSets(r.HostedZone)
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("name", "type", "ttl", "weight", "id", "hc id", "value")
	maxLen := 64
	for _, rrs := range rrsets {
		weight := ""
		if rrs.Weight > 0 {
			weight = fmt.Sprintf("%d", rrs.Weight)
		}
		col := []string{
			rrs.Name, rrs.Type, fmt.Sprintf("%d", rrs.TTL), rrs.SetIdentifier, weight, rrs.HealthCheckId,
		}
		for i, record := range rrs.ResourceRecords {
			v := record.Value
			if len(v) > maxLen {
				v = v[0:maxLen]
			}
			if i == 0 {
				col = append(col, v)
				table.AddStrings(col)
			} else {
				table.Add("", "", "", "", "", "", v)
			}
		}
	}
	fmt.Println(table)
	return nil
}
