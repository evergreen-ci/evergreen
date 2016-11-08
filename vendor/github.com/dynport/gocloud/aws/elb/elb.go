package elb

import (
	"encoding/xml"
	"fmt"
	"github.com/dynport/gocloud/aws"
	"log"
	"time"
)

const (
	API_VERSION = "2012-06-01"
)

type Client struct {
	*aws.Client
}

func NewFromEnv() *Client {
	return &Client{
		aws.NewFromEnv(),
	}
}

type DescribeLoadBalancersResponse struct {
	XMLName       xml.Name        `xml:"DescribeLoadBalancersResponse"`
	LoadBalancers []*LoadBalancer `xml:"DescribeLoadBalancersResult>LoadBalancerDescriptions>member"`
}

type LoadBalancer struct {
	LoadBalancerName          string    `"xml:"LoadBalancerName"`
	CreatedTime               time.Time `xml:"CreatedTime"`
	VPCId                     string    `xml:"VPCId"`
	CanonicalHostedZoneName   string    `xml:"CanonicalHostedZoneName"`
	CanonicalHostedZoneNameID string    `xml:"CanonicalHostedZoneNameID"`
	Scheme                    string    `xml:"Scheme"`
	DNSName                   string    `xml:"DNSName"`
	BackendServerDescriptions string    `xml:"BackendServerDescriptions"`

	HealthCheckInterval           int    `xml:"HealthCheck>Interval"`
	HealthCheckTarget             string `xml:"HealthCheck>Target"`
	HealthCheckHealthyThreshold   int    `xml:"HealthCheck>HealthyThreshold"`
	HealthCheckTimeout            int    `xml:"HealthCheck>Timeout"`
	HealthCheckUnhealthyThreshold int    `xml:"HealthCheck>UnhealthyThreshold"`

	SourceSecurityGroupOwnerAlias string `xml:"SourceSecurityGroup>OwnerAlias"`
	SourceSecurityGroupGroupName  string `xml:"SourceSecurityGroup>GroupName"`

	Listeners         []*Listener `xml:"ListenerDescriptions>member>Listener"`
	AvailabilityZones []string    `xml:"AvailabilityZones>member"`
	Instances         []string    `xml:"Instances>member>InstanceId"`
	Subnets           []string    `xml:"Subnets>member"`
}

type Listener struct {
	Protocol         string `xml:"Protocol"`
	LoadBalancerPort int    `xml:"LoadBalancerPort"`
	InstanceProtocol string `xml:"InstanceProtocol"`
	InstancePort     int    `xml:"InstancePort"`
}

type InstanceState struct {
	Description string `xml:"Description"`
	InstanceId  string `xml:"InstanceId"`
	State       string `xml:"State"`
	ReasonCode  string `xml:"ReasonCode"`
}

type DescribeInstanceHealthResponse struct {
	XMLName        xml.Name         `xml:"DescribeInstanceHealthResponse"`
	InstanceStates []*InstanceState `xml:"DescribeInstanceHealthResult>InstanceStates>member"`
}

func queryForAction(action string) string {
	return "Version=" + API_VERSION + "&Action=" + action
}

type RegisterInstancesWithLoadBalancerResponse struct {
	RequestId string `xml:"ResponseMetadata>RequestId"`
}

func (client *Client) Endpoint() string {
	prefix := "https://elasticloadbalancing"
	if client.Client.Region != "" {
		prefix += "." + client.Client.Region
	}
	return prefix + ".amazonaws.com"
}

func (client *Client) DescribeInstanceHealth(name string) (states []*InstanceState, e error) {
	query := queryForAction("DescribeInstanceHealth") + "&LoadBalancerName=" + name
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), query, nil)
	if e != nil {
		return nil, e
	}
	rsp := &DescribeInstanceHealthResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return states, e
	}
	return rsp.InstanceStates, nil
}

func (client *Client) DeregisterInstancesWithLoadBalancer(loadBalancerName string, instances []string) error {
	return client.updateLoadBalancerCall("DeregisterInstancesFromLoadBalancer", loadBalancerName, instances)
}

func (client *Client) RegisterInstancesWithLoadBalancer(loadBalancerName string, instances []string) error {
	log.Print("registering %#v with %s", instances, loadBalancerName)
	return client.updateLoadBalancerCall("RegisterInstancesWithLoadBalancer", loadBalancerName, instances)
}

func (client *Client) updateLoadBalancerCall(action string, loadBalancerName string, instances []string) error {
	query := queryForAction(action) + "&LoadBalancerName=" + loadBalancerName
	for i, id := range instances {
		query += fmt.Sprintf("&Instances.member.%d.InstanceId=%s", i+1, id)
	}
	log.Printf("sending request %s", query)
	raw, e := client.DoSignedRequest("POST", client.Endpoint(), query, nil)
	if e != nil {
		return e
	}
	log.Printf("status: %s", raw.StatusCode)
	log.Println(string(raw.Content))
	return nil
}

func (client *Client) DescribeLoadBalancers() (lbs []*LoadBalancer, e error) {
	raw, e := client.DoSignedRequest("GET", client.Endpoint(), "Version="+API_VERSION+"&Action=DescribeLoadBalancers", nil)
	if e != nil {
		return lbs, e
	}
	rsp := &DescribeLoadBalancersResponse{}
	e = xml.Unmarshal(raw.Content, rsp)
	if e != nil {
		return lbs, e
	}
	return rsp.LoadBalancers, e
}
