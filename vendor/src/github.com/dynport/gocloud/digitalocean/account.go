package digitalocean

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func NewAccount(clientId, apiKey string) *Account {
	return &Account{ApiKey: apiKey, ClientId: clientId}
}

type Account struct {
	Name     string
	ApiKey   string
	SshKey   int
	ClientId string

	RegionId int
	SizeId   int
	ImageId  int

	cachedSizes   map[int]string
	cachedImages  map[int]string
	cachedRegions map[int]string
}

func (a *Account) GetDroplet(id int) (*Droplet, error) {
	droplet := &Droplet{Id: id, Account: a}
	e := droplet.Reload()
	return droplet, e
}

type SshKeysResponse struct {
	Status string    `json:"status"`
	Keys   []*SshKey `json:"ssh_keys"`
}

func (account *Account) SshKeys() (keys []*SshKey, e error) {
	rsp := &SshKeysResponse{}
	e = account.loadResource("/ssh_keys", rsp, cacheFor(1*time.Hour))
	if e != nil {
		return keys, e
	}
	return rsp.Keys, nil
}

func cacheFor(dur time.Duration) *fetchOptions {
	return &fetchOptions{ttl: dur}
}

func (account *Account) Sizes() (sizes []*Size, e error) {
	rsp := &SizeResponse{}
	e = account.loadResource("/sizes", rsp, cacheFor(24*time.Hour))
	if e != nil {
		return sizes, e
	}
	return rsp.Sizes, nil
}

func (account *Account) CachedImages() (hash map[int]string, e error) {
	if account.cachedImages == nil {
		account.cachedImages = make(map[int]string)
		images, e := account.Images()
		if e != nil {
			return hash, e
		}
		for _, image := range images {
			account.cachedImages[image.Id] = image.Name
		}
	}
	return account.cachedImages, e
}

func (a *Account) RenameDroplet(id int, name string) (*EventResponse, error) {
	rsp := &EventResponse{}
	path := fmt.Sprintf("/droplets/%d/rename?name=%s", id, name)
	if e := a.loadResource(path, rsp, nil); e != nil {
		return nil, e
	}
	if rsp.Status != "OK" {
		err := "error renaming droplet: " + rsp.ErrorMessage
		logger.Error(err)
		return rsp, fmt.Errorf(err)
	}
	return rsp, nil
}

func (a *Account) RebuildDroplet(id int, imageId int) (*EventResponse, error) {
	if imageId == 0 {
		droplet := &Droplet{Id: id, Account: a}
		if e := droplet.Reload(); e != nil {
			return nil, e
		}
		imageId = droplet.ImageId
	}
	rsp := &EventResponse{}
	logger.Infof("rebuilding droplet %d and image %d", id, imageId)
	path := fmt.Sprintf("/droplets/%d/rebuild?image_id=%d", id, imageId)
	if e := a.loadResource(path, rsp, nil); e != nil {
		return nil, e
	}
	if rsp.Status != "OK" {
		err := "error rebuilding droplet: " + rsp.ErrorMessage
		logger.Error(err)
		return rsp, fmt.Errorf(err)
	}
	return rsp, nil
}

func (account *Account) DestroyDroplet(id int) (*EventResponse, error) {
	rsp := &EventResponse{}
	if e := account.loadResource(fmt.Sprintf("/droplets/%d/destroy", id), rsp, nil); e != nil {
		return nil, e
	}
	if rsp.Status != "OK" {
		return rsp, fmt.Errorf("error destroying droplet: %s", rsp.ErrorMessage)
	}
	return rsp, nil
}

func (account *Account) ImageName(i int) string {
	images, e := account.CachedImages()
	if e != nil {
		return ""
	}
	return images[i]
}

func (account *Account) CachedRegions() (hash map[int]string, e error) {
	if account.cachedRegions == nil {
		account.cachedRegions = make(map[int]string)
		regions, e := account.Regions()
		if e != nil {
			return hash, e
		}
		for _, region := range regions {
			account.cachedRegions[region.Id] = region.Name
		}
	}
	return account.cachedRegions, e
}

func (account *Account) RegionName(i int) string {
	regions, e := account.CachedRegions()
	if e != nil {
		return ""
	}
	return regions[i]
}

func (account *Account) CachedSizes() (hash map[int]string, e error) {
	if account.cachedSizes == nil {
		account.cachedSizes = make(map[int]string)
		sizes, e := account.Sizes()
		if e != nil {
			return hash, e
		}
		for _, size := range sizes {
			account.cachedSizes[size.Id] = size.Name
		}
	}
	return account.cachedSizes, e
}

func (account *Account) SizeName(i int) string {
	sizes, e := account.CachedSizes()
	if e != nil {
		return ""
	}
	return sizes[i]
}

func (account *Account) DefaultDroplet() (droplet *Droplet) {
	droplet = &Droplet{}
	if account.RegionId > 0 {
		droplet.RegionId = account.RegionId
	}
	if account.SizeId > 0 {
		droplet.SizeId = account.SizeId
	}
	if account.ImageId > 0 {
		droplet.ImageId = account.ImageId
	}
	return droplet
}

type ErrorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error_message"`
}

func (account *Account) urlFor(path string) string {
	url := fmt.Sprintf("%s%s", API_ROOT, path)
	if !strings.Contains(url, "?") {
		url += "?"
	} else {
		url += "&"
	}
	return url + fmt.Sprintf("client_id=%s&api_key=%s", account.ClientId, account.ApiKey)
}

type fetchOptions struct {
	ttl time.Duration
}

type GenericResponse struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
}

func (account *Account) loadResource(path string, i interface{}, opts *fetchOptions) (e error) {
	url := account.urlFor(path)
	r := &resource{
		Url: url,
	}
	if e := r.load(opts); e != nil {
		return e
	}
	gr := &GenericResponse{}
	e = json.Unmarshal(r.Content, gr)
	if e != nil {
		return e
	}
	if gr.Status != "OK" {
		return fmt.Errorf(gr.ErrorMessage)
	}

	e = json.Unmarshal(r.Content, i)
	if e != nil {
		logger.Error(string(r.Content))
	}
	return e
}

func (self *Account) Droplets() (droplets []*Droplet, e error) {
	dropletResponse := &DropletsResponse{}
	e = self.loadResource("/droplets", dropletResponse, cacheFor(10*time.Second))
	if e != nil {
		return droplets, e
	}
	droplets = dropletResponse.Droplets
	for i := range droplets {
		droplets[i].Account = self
	}
	return droplets, nil
}
