package elasticache

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/dynport/gocloud/aws"
)

type Client struct {
	*aws.Client
}

func (client *Client) Endpoint() (string, error) {
	if client.Client.Region == "" {
		return "", fmt.Errorf("Region must be set (e.g. in AWS_DEFAULT_REGION")
	}
	return fmt.Sprintf("https://elasticache.%s.amazonaws.com", client.Client.Region), nil
}

func NewFromEnv() *Client {
	return &Client{
		Client: aws.NewFromEnv(),
	}
}

type DescribeCacheClusters struct {
	CacheClusterId    string `xml:",omitempty"`
	Marker            string `xml:",omitempty"`
	MaxRecords        int    `xml:",omitempty"`
	ShowCacheNodeInfo bool   `xml:",omitempty"`
}

type DescribeCacheClustersResponse struct {
	DescribeCacheClustersResult *DescribeCacheClustersResult `xml:",omitempty"`
	ResponseMetadata            *ResponseMetadata            `xml:",omitempty"`
}

type ResponseMetadata struct {
	RequestId string `xml:",omitempty"`
}

type DescribeCacheClustersResult struct {
	CacheClusters []*CacheCluster `xml:"CacheClusters>CacheCluster,omitempty"`
	Marker        string          `xml:",omitempty"`
}

type values map[string]string

func (m values) Encode() string {
	uv := url.Values{}
	for k, v := range m {
		if v != "" {
			uv.Set(k, v)
		}
	}
	return uv.Encode()
}

func (c *DescribeCacheClusters) Execute(client *Client) (*DescribeCacheClustersResponse, error) {
	vals := values{
		"Version":           "2014-03-24",
		"Action":            "DescribeCacheClusters",
		"CacheClusterId":    c.CacheClusterId,
		"ShowCacheNodeInfo": fmt.Sprintf("%t", c.ShowCacheNodeInfo),
	}
	if c.MaxRecords > 0 {
		vals["MaxRecords"] = strconv.Itoa(c.MaxRecords)
	}
	endpoint, e := client.Endpoint()
	if e != nil {
		return nil, e
	}

	httpResponse, e := client.DoSignedRequest("GET", endpoint, vals.Encode(), nil)
	if e != nil {
		return nil, e
	}
	dbg.Printf("%s", string(httpResponse.Content))
	rsp := &DescribeCacheClustersResponse{}
	e = xml.Unmarshal(httpResponse.Content, rsp)
	return rsp, e
}

type CacheCluster struct {
	AutoMinorVersionUpgrade    bool                            `xml:",omitempty"`
	CacheClusterCreateTime     time.Time                       `xml:",omitempty"`
	CacheClusterId             string                          `xml:",omitempty"`
	CacheClusterStatus         string                          `xml:",omitempty"`
	CacheNodeType              string                          `xml:",omitempty"`
	CacheNodes                 []*CacheNode                    `xml:"CacheNodes>CacheNode,omitempty"`
	CacheParameterGroup        *CacheParameterGroupStatus      `xml:",omitempty"`
	CacheSecurityGroups        []*CacheSecurityGroupMembership `xml:",omitempty"`
	CacheSubnetGroupName       string                          `xml:",omitempty"`
	ClientDownloadLandingPage  string                          `xml:",omitempty"`
	ConfigurationEndpoint      *Endpoint                       `xml:",omitempty"`
	Engine                     string                          `xml:",omitempty"`
	EngineVersion              string                          `xml:",omitempty"`
	NotificationConfiguration  *NotificationConfiguration      `xml:",omitempty"`
	NumCacheNodes              int                             `xml:",omitempty"`
	PendingModifiedValues      *PendingModifiedValues          `xml:",omitempty"`
	PreferredAvailabilityZone  string                          `xml:",omitempty"`
	PreferredMaintenanceWindow string                          `xml:",omitempty"`
	ReplicationGroupId         string                          `xml:",omitempty"`
	SecurityGroups             []*SecurityGroupMembership      `xml:",omitempty"`
	SnapshotRetentionLimit     int                             `xml:",omitempty"`
	SnapshotWindow             string                          `xml:",omitempty"`
}

type SecurityGroupMembership struct {
	SecurityGroupId string `xml:",omitempty"`
	Status          string `xml:",omitempty"`
}

type PendingModifiedValues struct {
	CacheNodeIdsToRemove []string `xml:",omitempty"`
	EngineVersion        string   `xml:",omitempty"`
	NumCacheNodes        int      `xml:",omitempty"`
}

type CacheNode struct {
	CacheNodeCreateTime  time.Time `xml:",omitempty"`
	CacheNodeId          string    `xml:",omitempty"`
	CacheNodeStatus      string    `xml:",omitempty"`
	Endpoint             *Endpoint `xml:",omitempty"`
	ParameterGroupStatus string    `xml:",omitempty"`
	SourceCacheNodeId    string    `xml:",omitempty"`
}

type Endpoint struct {
	Address string `xml:",omitempty"`
	Port    int    `xml:",omitempty"`
}

type CacheParameterGroupStatus struct {
	CacheNodeIdsToReboot    []string `xml:",omitempty"`
	CacheParameterGroupName string   `xml:",omitempty"`
	ParameterApplyStatus    string   `xml:",omitempty"`
}

type CacheSecurityGroupMembership struct {
	CacheSecurityGroupName string `xml:",omitempty"`
	Status                 string `xml:",omitempty"`
}

type NotificationConfiguration struct {
	TopicArn    string `xml:",omitempty"`
	TopicStatus string `xml:",omitempty"`
}
