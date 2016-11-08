package digitalocean

import (
	"fmt"
	"github.com/dynport/gologger"
	"strconv"
	"time"
)

type DropletResponse struct {
	Status   string `json:"status"`
	*Droplet `json:"droplet"`
}

type DropletsResponse struct {
	Status   string     `json:"status"`
	Droplets []*Droplet `json:"droplets"`
}

type Droplet struct {
	Id        int       `json:"id"`
	ImageId   int       `json:"image_id"`
	SizeId    int       `json:"size_id"`
	RegionId  int       `json:"region_id"`
	Name      string    `json:"name"`
	IpAddress string    `json:"ip_address"`
	Locked    bool      `json:"locked"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	SshKey    int
	*Account
}

func (droplet *Droplet) Reload() error {
	account := droplet.Account
	if account == nil {
		return fmt.Errorf("account not set")
	}
	dropletResponse := &DropletResponse{}
	dropletResponse.Droplet = droplet
	e := droplet.Account.loadResource("/droplets/"+strconv.Itoa(droplet.Id), dropletResponse, nil)
	if e != nil {
		return e
	}
	droplet = dropletResponse.Droplet
	droplet.Account = account
	return nil
}

func WaitForDroplet(droplet *Droplet) error {
	started := time.Now()
	logger.Infof("waiting for droplet %d", droplet.Id)
	level := logger.LogLevel
	defer func(level int) {
		logger.LogLevel = level
	}(level)
	logger.LogLevel = gologger.WARN
	for i := 0; i < 60; i++ {
		if e := droplet.Reload(); e != nil {
			return e
		}
		if droplet.Status == "active" && droplet.Locked == false {
			break
		}
		fmt.Print(".")
		time.Sleep(5 * time.Second)
	}
	fmt.Print("\n")
	logger.Debugf("waited for %.06f", time.Now().Sub(started).Seconds())
	return nil
}

type EventResponse struct {
	Status       string `json:"status"`
	EventId      int    `json:"event_id"`
	ErrorMessage string `json:"error_message"`
}

func (account *Account) CreateDroplet(droplet *Droplet) (out *Droplet, e error) {
	rsp := &DropletResponse{}
	path := fmt.Sprintf("/droplets/new?name=%s&size_id=%d&image_id=%d&region_id=%d", droplet.Name, droplet.SizeId, droplet.ImageId, droplet.RegionId)
	if droplet.SshKey > 0 {
		path += "&ssh_key_ids=" + strconv.Itoa(droplet.SshKey)
	}
	e = account.loadResource(path, rsp, nil)
	return rsp.Droplet, e
}
