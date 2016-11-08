package aws

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"
)

type awsCredentials struct {
	Code            string    `json:"Code,omitempty"`
	LastUpdated     time.Time `json:"LastUpdated,omitempty"`
	Type            string    `json:"Type,omitempty"`
	AccessKeyId     string    `json:"AccessKeyId,omitempty"`
	SecretAccessKey string    `json:"SecretAccessKey,omitempty"`
	Token           string    `json:"Token,omitempty"`
	Expiration      time.Time `json:"Expiration,omitempty"`
}

func CurrentAwsRegion() (string, error) {
	s, e := CurrentAwsAvailabilityZone()
	if e != nil {
		return "", e
	}
	if len(s) > 4 {
		return s[0 : len(s)-1], nil
	}
	return "", nil
}

func CurrentAwsAvailabilityZone() (string, error) {
	rsp, e := http.Get("http://169.254.169.254/latest/meta-data/placement/availability-zone")
	if e != nil {
		return "", e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	return string(b), e
}

func loadAwsCredentials(theUrl string) (*awsCredentials, error) {
	r := &awsCredentials{}
	rsp, e := http.Get(theUrl)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(b, r)
	return r, e
}

func newFromIam() (*Client, error) {
	con, e := net.DialTimeout("tcp", metadataIp+":80", 10*time.Millisecond)
	if e != nil {
		return nil, e
	}
	defer con.Close()

	rsp, e := http.Get(securityCredentialsUrl)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()
	if rsp.Status[0] != '2' {
		return nil, fmt.Errorf("expected status 2xx, got %s", rsp.Status)
	}

	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return nil, e
	}

	rolesString := string(b)
	if rolesString == "" {
		return nil, nil
	}

	roles := strings.Fields(string(b))
	if len(roles) > 0 {
		role := roles[0]
		r, e := loadAwsCredentials(securityCredentialsUrl + role)
		if e != nil {
			return nil, e
		}
		client := &Client{Key: r.AccessKeyId, Secret: r.SecretAccessKey, SecurityToken: r.Token}
		client.Region, _ = CurrentAwsRegion()
		return client, nil
	}
	return nil, nil
}
