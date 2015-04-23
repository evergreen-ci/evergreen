package jiffybox

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	STATUS_START    = "START"
	STATUS_SHUTDOWN = "SHUTDOWN"
	STATUS_PULLPLUG = "PULLPLUG"
	STATUS_FREEZE   = "FREEZE"
	STATUS_THAW     = "THAW"
)

type Disk struct {
	Name       string `json:"name"`       //" Swap",
	Filesystem string `json:"filesystem"` // "swap",
	SizeInMB   int    `json:"sizeInMB"`   // 512,
	Created    int    `json:"created"`    // 1234567890,
	Status     string `json:"status"`     // "READY"
}

type ActiveProfile struct {
	Name         string           `json:"name"`         // "Standard",
	Created      int              `json:"created"`      // 1234567890,
	Runlevel     string           `json:"runlevel"`     // "default",
	Kernel       string           `json:"kernel"`       // "xen-current",
	Rootdisk     string           `json:"rootdisk"`     // "\/dev\/xvda",
	RootdiskMode string           `json:"rootdiskMode"` // "ro",
	Status       string           `json:"status"`       // "READY",
	DisksHash    map[string]*Disk `json:"disks"`
}

func (ap *ActiveProfile) Disks() (disks []*Disk) {
	for _, disk := range ap.DisksHash {
		disks = append(disks, disk)
	}
	return disks

}

type Server struct {
	Id                  int                 `json:"id"`
	Name                string              `json:"name"`
	Ips                 map[string][]string `json:"ips"`
	Status              string              `json:"status"`
	Created             int64               `json:"created"`
	RecoverymodeActive  bool                `json:"recoverymodeActive"`
	ManualBackupRunning bool                `json:"manualBackupRunning"`
	IsBeingCopied       bool                `json:"isBeingCopied"`
	Running             bool                `json:"running"`
	Host                string              `json:"host"`
	Plan                *Plan               `json:"plan"`
	ActiveProfile       *ActiveProfile      `json:"activeProfile"`
	Metadata            map[string]string   `json:"metadata"`
}

func (server *Server) IpsString() string {
	ips := []string{}
	for k, v := range server.Ips {
		ips = append(ips, k+":"+strings.Join(v, ","))
	}
	return strings.Join(ips, ", ")
}

func (server *Server) CreatedAt() time.Time {
	return time.Unix(server.Created, 0)
}

func (server *Server) PublicIp() string {
	if ips := server.Ips["public"]; len(ips) > 0 {
		return ips[0]
	}
	return ""
}

func (server *Server) Frozen() bool {
	return server.Status == "FROZEN" || server.Status == "FREEZING"
}

type Message struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type JiffyBoxResponse struct {
	Messages []*Message
	Server   *Server `json:"result"`
}

type JiffyBoxesResponse struct {
	Messages   []*Message         `json:"messages:"`
	ServersMap map[string]*Server `json:"result"`
}

func (rsp *JiffyBoxesResponse) Servers() []*Server {
	servers := make([]*Server, 0, len(rsp.ServersMap))
	for _, server := range rsp.ServersMap {
		servers = append(servers, server)
	}
	return servers
}

func (rsp *JiffyBoxesResponse) Server() *Server {
	for _, server := range rsp.ServersMap {
		return server
	}
	return nil
}

func (client *Client) JiffyBoxes() (servers []*Server, e error) {
	rsp := &JiffyBoxesResponse{}
	e = client.LoadResource("jiffyBoxes", rsp)
	if e != nil {
		return servers, e
	}
	return rsp.Servers(), nil
}

func (client *Client) JiffyBox(id int) (server *Server, e error) {
	rsp := &JiffyBoxResponse{}
	e = client.LoadResource("jiffyBoxes/"+strconv.Itoa(id), rsp)
	if e != nil {
		return server, e
	}
	return rsp.Server, nil
}

func (client *Client) DeleteJiffyBox(id int) (e error) {
	req, e := http.NewRequest("DELETE", client.BaseUrl()+"/jiffyBoxes/"+strconv.Itoa(id), nil)
	if e != nil {
		return e
	}
	httpResponse, e := http.DefaultClient.Do(req)
	if e != nil {
		return e
	}
	rsp := &Response{}
	e = client.unmarshalResponse(httpResponse, rsp)
	if e != nil {
		return e
	}
	return nil
}

type CreateOptions struct {
	Name         string
	PlanId       int
	BackupId     string
	Distribution string
	Password     string
	UseSshKey    bool
	Metadata     string
}

func (client *Client) CreateJiffyBox(options *CreateOptions) (server *Server, e error) {
	values := url.Values{}
	values.Add("name", options.Name)
	values.Add("planid", strconv.Itoa(options.PlanId))
	if options.BackupId != "" {
		values.Add("backupid", options.BackupId)
	} else if options.Distribution != "" {
		values.Add("distribution", options.Distribution)
	}
	if options.Password != "" {
		values.Add("password", options.Password)
	}
	if options.UseSshKey {
		values.Add("use_sshkey", "true")
	}
	if options.Metadata != "" {
		values.Add("metadata", options.Metadata)
	}
	httpResponse, e := http.PostForm(client.BaseUrl()+"/jiffyBoxes", values)
	if e != nil {
		return nil, e
	}
	rsp := &JiffyBoxResponse{}
	e = client.unmarshalResponse(httpResponse, rsp)
	return rsp.Server, e
}

// unfreeze server
func (client *Client) ThawServer(id int, planId int) (server *Server, e error) {
	return client.ChangeState(id, STATUS_THAW, planId)
}

func (client *Client) FreezeServer(id int) (server *Server, e error) {
	return client.ChangeState(id, STATUS_FREEZE, -1)
}

func (client *Client) PullPlugServer(id int) (server *Server, e error) {
	return client.ChangeState(id, STATUS_PULLPLUG, -1)
}

func (client *Client) ShutdownServer(id int) (server *Server, e error) {
	return client.ChangeState(id, STATUS_SHUTDOWN, -1)
}

func (client *Client) StartServer(id int, planId int) (server *Server, e error) {
	return client.ChangeState(id, STATUS_START, planId)
}

func (client *Client) ChangeState(id int, state string, planId int) (server *Server, e error) {
	values := url.Values{}
	values.Add("status", state)
	if planId > 0 {
		values.Add("planid", strconv.Itoa(planId))
	}
	buf := bytes.Buffer{}
	buf.WriteString(values.Encode())
	req, e := http.NewRequest("PUT", client.BaseUrl()+"/jiffyBoxes/"+strconv.Itoa(id), &buf)
	if e != nil {
		return nil, e
	}
	httpResponse, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	rsp := &JiffyBoxResponse{}
	e = client.unmarshalResponse(httpResponse, rsp)
	if e != nil {
		return rsp.Server, e
	}
	errors := []string{}
	for _, message := range rsp.Messages {
		if message.Type == "error" {
			errors = append(errors, message.Message)
		}
	}
	if len(errors) > 0 {
		return rsp.Server, fmt.Errorf(strings.Join(errors, ", "))
	}
	return rsp.Server, e
}

func (client *Client) CloneServer(id int, opts *CreateOptions) (server *Server, e error) {
	values := url.Values{}
	if opts.Name == "" {
		return nil, fmt.Errorf("name must be provided to clone server")
	}
	values.Add("name", opts.Name)
	values.Add("planid", strconv.Itoa(opts.PlanId))
	if opts.Metadata != "" {
		values.Add("metadata", opts.Metadata)
	}
	httpResponse, e := client.PostForm("jiffyBoxes/"+strconv.Itoa(id), values)
	if e != nil {
		return nil, e
	}
	rsp := &JiffyBoxResponse{}
	e = client.unmarshal(httpResponse.Content, rsp)
	if e != nil {
		return nil, e
	}
	return rsp.Server, nil
}

func (client *Client) ChangeStatus(id string, status string, planId int, metadata string) (server *Server, e error) {
	values := url.Values{}
	values.Add("status", status)
	values.Add("planid", strconv.Itoa(planId))
	if metadata != "" {
		values.Add("metadata", metadata)
	}
	buf := &bytes.Buffer{}
	buf.Write([]byte(values.Encode()))
	req, e := http.NewRequest("PUT", client.BaseUrl()+"/JiffyBox/"+id, buf)
	if e != nil {
		return nil, e
	}
	httpResponse, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	rsp := &JiffyBoxResponse{}
	e = client.unmarshalResponse(httpResponse, rsp)
	if e != nil {
		return nil, e
	}
	return rsp.Server, nil
}
