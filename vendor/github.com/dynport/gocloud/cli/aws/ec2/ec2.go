package ec2

import (
	"fmt"
	"io/ioutil"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/aws/ec2"
)

const HOURS_PER_MONTH = 365 * 24.0 / 12.0

func Register(router *cli.Router) {
	router.Register("aws/ec2/instances/describe", &DescribeInstances{}, "Describe ec2 instances")
	router.Register("aws/ec2/instances/run", &RunInstances{}, "Run ec2 instances")
	router.Register("aws/ec2/images/create", &CreateImage{}, "Create image from instance")
	router.Register("aws/ec2/instances/terminate", &TerminateInstances{}, "Terminate ec2 instances")
	router.Register("aws/ec2/tags/create", &CreateTags{}, "Create Tags")
	router.RegisterFunc("aws/ec2/tags/describe", DescribeTags, "Describe Tags")
	router.Register("aws/ec2/images/describe", &DescribeImages{}, "Describe ec2 Images")
	router.RegisterFunc("aws/ec2/key-pairs/describe", DescribeKeyPairs, "Describe key pairs")
	router.RegisterFunc("aws/ec2/addresses/describe", DescribeAddresses, "Describe Addresses")
	router.RegisterFunc("aws/ec2/security-groups/describe", DescribeSecurityGroups, "Describe Security Groups")
	router.RegisterFunc("aws/ec2/spot-price-history/describe", DescribeSpotPriceHistory, "Describe Spot Price History")
	//router.Register("aws/ec2/prices", &Prices{Region: os.Getenv("AWS_DEFAULT_REGION")}, "EC2 Prices")
}

func client() *ec2.Client {
	return ec2.NewFromEnv()
}

// http://docs.aws.amazon.com/general/latest/gr/rande.html#ec2_region
var regionMapping = map[string]string{
	"eu-ireland": "eu-west-1",
	"eu-west":    "eu-west-1",
	"apac-tokyo": "ap-northeast-1",
	"apac-sin":   "ap-southeast-1",
	"apac-syd":   "ap-southeast-2",
}

// http://docs.aws.amazon.com/general/latest/gr/rande.html#ec2_region
var regionMappingForOd = map[string]string{
	"eu-west-1":      "eu-ireland",
	"us-east-1":      "us-east",
	"us-west-1":      "us-west",
	"ap-northeast-1": "apac-tokyo",
	"ap-southeast-1": "apac-sin",
	"ap-southeast-2": "apac-syd",
}

func normalizeRegionForOd(region string) string {
	normalized, ok := regionMappingForOd[region]
	if ok {
		return normalized
	}
	return region
}

func normalizeRegion(raw string) string {
	if v, ok := regionMapping[raw]; ok {
		return v
	}
	return raw
}
func monthlyPrice(price float64) string {
	return fmt.Sprintf("%.02f", price)
}

type DescribeImages struct {
	Canonical bool `cli:"type=opt long=canonical"`
	Self      bool `cli:"type=opt long=self"`
	UbuntuAll bool `cli:"opt --ubuntu-all"`
	Ubuntu    bool `cli:"type=opt long=ubuntu"`
	Raring    bool `cli:"type=opt long=raring"`
	Saucy     bool `cli:"type=opt long=saucy"`
	Trusty    bool `cli:"type=opt long=trusty"`
	Ssd       bool `cli:"opt --ssd"`
	HvmSsd    bool `cli:"opt --hvm-ssd"`
}

func (a *DescribeImages) Run() error {
	filter := &ec2.ImageFilter{}
	if a.Canonical {
		filter.Owner = ec2.CANONICAL_OWNER_ID
	} else if a.Self {
		filter.Owner = ec2.SELF_OWNER_ID
	}

	storageType := "*"
	if a.Ssd {
		storageType = "ebs-ssd"
	} else if a.HvmSsd {
		storageType = "hvm-ssd"
	}

	if a.UbuntuAll {
		filter.Name = ec2.UBUNTU_ALL
	} else if a.Ubuntu {
		filter.Name = "ubuntu/images/" + storageType + "/" + ec2.UBUNTU_PREFIX
	} else if a.Raring {
		filter.Name = "ubuntu/images/" + storageType + "/" + ec2.UBUNTU_RARING_PREFIX
	} else if a.Saucy {
		filter.Name = "ubuntu/images/" + storageType + "/" + ec2.UBUNTU_SAUCY_PREFIX
	} else if a.Trusty {
		filter.Name = "ubuntu/images/" + storageType + "/" + ec2.UBUNTU_TRUSTY_PREFIX
	}
	log.Printf("describing images with filter %q", filter.Name)

	images, e := client().DescribeImagesWithFilter(filter)
	if e != nil {
		return e
	}
	sort.Sort(images)
	table := gocli.NewTable()
	for _, image := range images {
		table.Add(image.ImageId, image.Name, image.ImageState, image.Hypervisor, image.VirtualizationType)
	}
	fmt.Println(table)
	return nil
}

func DescribeSpotPriceHistory() error {
	filter := &ec2.SpotPriceFilter{
		InstanceTypes:       []string{"c1.medium"},
		ProductDescriptions: []string{ec2.DESC_LINUX_UNIX},
		StartTime:           time.Now().Add(-7 * 24 * time.Hour),
	}
	prices, e := client().DescribeSpotPriceHistory(filter)
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	for _, price := range prices {
		table.Add(price.InstanceType, price.ProductDescription, price.SpotPrice, price.Timestamp, price.AvailabilityZone)
	}
	fmt.Println(table)
	return nil
}

func DescribeSecurityGroups() error {
	groups, e := client().DescribeSecurityGroups(nil)
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	for _, group := range groups {
		table.Add(group.GroupId, "", "", group.GroupName)
		for _, perm := range group.IpPermissions {
			ports := ""
			if perm.FromPort > 0 {
				if perm.FromPort == perm.ToPort {
					ports += fmt.Sprintf("%d", perm.FromPort)
				} else {
					ports += fmt.Sprintf("%d-%d", perm.FromPort, perm.ToPort)
				}
			}
			groups := []string{}
			for _, group := range perm.Groups {
				groups = append(groups, group.GroupId)
			}
			if len(groups) > 0 {
				table.Add("", perm.IpProtocol, ports, strings.Join(groups, ","))
			}

			if len(perm.IpRanges) > 0 {
				table.Add("", perm.IpProtocol, ports, strings.Join(perm.IpRanges, ","))
			}
		}
	}
	fmt.Print(table)
	return nil
}

func DescribeAddresses() error {
	addresses, e := client().DescribeAddresses()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	for _, address := range addresses {
		table.Add(address.PublicIp, address.PrivateIpAddress, address.Domain, address.InstanceId)
	}
	fmt.Print(table)
	return nil
}

func DescribeKeyPairs() error {
	pairs, e := client().DescribeKeyPairs()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	for _, pair := range pairs {
		table.Add(pair.KeyName, pair.KeyFingerprint)
	}
	fmt.Print(table)
	return nil
}

type CreateImage struct {
	ImageId string `cli:"type=arg required=true"`
}

func (a *CreateImage) Run() error {
	log.Printf("creating image of instance %s", a.ImageId)
	return fmt.Errorf("implement me")
}

type RunInstances struct {
	Name             string `cli:"type=opt long=name desc='Name of the Instanze'"`
	InstanceType     string `cli:"type=opt short=t desc='Instance Type' required=true"`
	ImageId          string `cli:"type=opt short=i desc='Image Id' required=true"`
	KeyName          string `cli:"type=opt short=k desc='SSH Key' required=true"`
	SecurityGroup    string `cli:"type=opt short=g desc='Security Group'"`
	SubnetId         string `cli:"type=opt long=subnet-id desc='Subnet Id'"`
	PublicIp         bool   `cli:"type=opt long=public-ip desc='Assign Public IP'"`
	AvailabilityZone string `cli:"type=opt long=availability-zone desc='Availability Zone'"`
	UserDataFile     string `cli:"type=opt long=userdata desc='Path to file with userdata'"`
}

func (a *RunInstances) Run() error {
	config := &ec2.RunInstancesConfig{
		ImageId:          a.ImageId,
		KeyName:          a.KeyName,
		InstanceType:     a.InstanceType,
		AvailabilityZone: a.AvailabilityZone,
		SubnetId:         a.SubnetId,
	}
	if a.UserDataFile != "" {
		b, e := ioutil.ReadFile(a.UserDataFile)
		if e != nil {
			return e
		}
		config.UserData = string(b)
	}
	if a.PublicIp {
		config.AddPublicIp()
	} else {
		if a.SecurityGroup != "" {
			config.SecurityGroups = []string{a.SecurityGroup}
		}
	}
	list, e := client().RunInstances(config)
	ids := []string{}
	for _, i := range list {
		ids = append(ids, i.InstanceId)
	}
	if a.Name != "" {
		log.Printf("tagging %v with %q", ids, a.Name)
		e := client().CreateTags(ids, map[string]string{"Name": a.Name})
		if e != nil {
			log.Printf("ERROR: " + e.Error())
		}
	}
	log.Printf("started instances %v", ids)
	return e
}

type TerminateInstances struct {
	InstanceIds []string `cli:"type=arg required=true"`
}

func (a *TerminateInstances) Run() error {
	_, e := client().TerminateInstances(a.InstanceIds)
	return e
}

type CreateTags struct {
	ResourceId string `cli:"type=arg required=true"`
	Key        string `cli:"type=arg required=true"`
	Value      string `cli:"type=arg required=true"`
}

func (r *CreateTags) Run() error {
	tags := map[string]string{
		r.Key: r.Value,
	}
	return client().CreateTags([]string{r.ResourceId}, tags)
}

func DescribeTags() error {
	tags, e := client().DescribeTags()
	if e != nil {
		return e
	}
	sort.Sort(tags)
	table := gocli.NewTable()
	for _, tag := range tags {
		table.Add(tag.ResourceType, tag.ResourceId, tag.Key, tag.Value)
	}
	fmt.Println(table)
	return nil
}

type DescribeInstances struct {
	Detailed bool `cli:"type=opt long=detailed"`
}

func (a *DescribeInstances) Run() error {
	log.Print("describing ec2 instances")
	instances, e := client().DescribeInstances()
	if e != nil {
		return e
	}
	if a.Detailed {
		for _, i := range instances {
			table := gocli.NewTable()
			table.Add(i.InstanceId, "Name", i.Name())
			table.Add(i.InstanceId, "ImageId", i.ImageId)
			table.Add(i.InstanceId, "InstanceType", i.InstanceType)
			table.Add(i.InstanceId, "InstanceStateName", i.InstanceStateName)
			table.Add(i.InstanceId, "Availability Zone", i.PlacementAvailabilityZone)
			table.Add(i.InstanceId, "KeyName", i.KeyName)
			table.Add(i.InstanceId, "IpAddress", i.IpAddress)
			table.Add(i.InstanceId, "PrivateIpAddress", i.PrivateIpAddress)
			table.Add(i.InstanceId, "LaunchTime", i.LaunchTime.Format("2006-01-02T15:04:05"))
			table.Add(i.InstanceId, "VpcId", i.VpcId)
			table.Add(i.InstanceId, "MonitoringState", i.MonitoringState)
			table.Add(i.InstanceId, "SubnetId", i.SubnetId)
			for _, group := range i.SecurityGroups {
				table.Add(i.InstanceId, "SecurityGroup", group.GroupId)
			}
			for _, tag := range i.Tags {
				table.Add(i.InstanceId, "Tag", tag.Key, tag.Value)
			}
			fmt.Println(table)
		}
		return nil
	}
	table := gocli.NewTable()
	table.Add("id", "image", "name", "state", "type", "private_ip", "ip", "az", "launched")
	for _, i := range instances {
		sgs := []string{}
		if i.InstanceStateName != "running" {
			continue
		}
		for _, g := range i.SecurityGroups {
			sgs = append(sgs, g.GroupId)
		}
		table.Add(
			i.InstanceId,
			i.ImageId,
			i.Name(),
			i.InstanceStateName,
			i.InstanceType,
			i.PrivateIpAddress,
			i.IpAddress,
			i.PlacementAvailabilityZone,
			i.LaunchTime.Format("2006-01-02T15:04"),
		)
	}
	fmt.Println(table)
	return nil
}
