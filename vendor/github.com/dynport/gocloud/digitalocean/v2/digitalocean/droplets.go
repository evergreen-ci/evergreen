package digitalocean

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type DropletsResponse struct {
	Droplets []*Droplet `json:"droplets"`
	Meta     *Meta      `json:"neta,omitempty"`
}

type DropletResponse struct {
	Droplet *Droplet `json:"droplet,omitempty"`
}

func (client *Client) DropletDelete(idOrSlug string) error {
	req, e := http.NewRequest("DELETE", root+"/v2/droplets/"+idOrSlug, nil)
	if e != nil {
		return e
	}
	rsp, e := client.Do(req)
	if e != nil {
		return e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return e
	}
	if rsp.Status[0] != '2' {
		return fmt.Errorf("expected status 2xx, got %s: %s", rsp.Status, string(b))
	}
	return nil
}

func (client *Client) Droplets() (*DropletsResponse, error) {
	rsp := &DropletsResponse{}
	e := client.loadResponse("/v2/droplets", rsp)
	return rsp, e
}

func (client *Client) Droplet(idOrSlug string) (*DropletResponse, error) {
	rsp := &DropletResponse{}
	e := client.loadResponse("/v2/droplets/"+idOrSlug, rsp)
	return rsp, e
}

type CreateDroplet struct {
	Name              string   `json:"name,omitempty"`   // required
	Region            string   `json:"region,omitempty"` // required
	Size              string   `json:"size,omitempty"`   // required
	Image             string   `json:"image,omitempty"`  // required
	SshKeys           []string `json:"ssh_keys,omitempty"`
	Backups           bool     `json:"backups,omitempty"`
	IPv6              bool     `json:"ipv6,omitempty"`
	PrivateNetworking bool     `json:"private_networking,omitempty"`
}

func (c *CreateDroplet) Execute(client *Client) (*DropletResponse, error) {
	b, e := json.Marshal(c)
	if e != nil {
		return nil, e
	}

	dbg.Printf("creating image with %s", string(b))

	req, e := http.NewRequest("POST", root+"/v2/droplets", bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	dbg.Printf("URL %s", req.URL.String())
	for k, v := range req.Header {
		dbg.Printf("HEADER %s: %s", k, v[0])
	}
	rsp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()
	b, e = ioutil.ReadAll(rsp.Body)
	if e != nil {
		return nil, e
	}
	if rsp.Status[0] != '2' {
		return nil, fmt.Errorf("expected status 2xx, got %s: %s", rsp.Status, string(b))
	}
	r := &DropletResponse{}
	e = json.Unmarshal(b, r)
	return r, e

}

type Meta struct {
	Total int `json:"total,omitempty"`
}

type Droplet struct {
	Id          int       `json:"id,omitempty"`
	Name        string    `json:"name,omitempty"`
	Region      *Region   `json:"region,omitempty"`
	Image       *Image    `json:"image,omitempty"`
	Size        *Size     `json:"size,omitempty"`
	Locked      bool      `json:"locked,omitempty"`
	Status      string    `json:"status,omitempty"`
	Networks    *Networks `json:"networks,omitempty"`
	Kernel      *Kernel   `json:"kernel,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	BackupIds   []int64   `json:"backup_ids,omitempty"`
	SnapshotIds []int64   `json:"snapshot_ids,omitempty"`
	ActionIds   []int64   `json:"action_ids,omitempty"`
}

type Networks struct {
	V4 []*Network `json:"v4,omitempty"`
	V6 []*Network `json:"v6,omitempty"`
}

type Kernel struct {
	Id      int64  `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type Network struct {
	IpAddress string `json:"ip_address,omitempty"`
	Netmask   string `json:"netmask,omitempty"`
	Gateway   string `json:"gateway,omitempty"`
	Type      string `json:"type,omitempty"`
}

type Region struct {
	Slug      string   `json:"slug,omitempty"`
	Name      string   `json:"name,omitempty"`
	Sizes     []string `json:"sizes,omitempty"`
	Available bool     `json:"available,omitempty"`
	Features  []string `json:"features,omitempty"`
}

type Image struct {
	Id           int       `json:"id,omitempty"`
	Name         string    `json:"name,omitempty"`
	Distribution string    `json:"distribution,omitempty"`
	Slug         string    `json:"slug,omitempty"`
	Public       bool      `json:"public,omitempty"`
	Regions      []string  `json:"regions,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
}

type Size struct {
	Slug          string      `json:"slug,omitempty"`
	Memory        int         `json:"memory,omitempty"`
	VCpus         int         `json:"v_cpus,omitempty"`
	Disk          int         `json:"disk,omitempty"`
	Transfer      interface{} `json:"transfer,omitempty"`
	PriceMonthley float64     `json:"price_monthley,omitempty"`
	PriceHourly   float64     `json:"price_hourly,omitempty"`
	Regions       []string    `json:"regions,omitempty"`
}
