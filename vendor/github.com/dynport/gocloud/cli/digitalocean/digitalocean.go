package digitalocean

import (
	"fmt"
	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/digitalocean"
	"github.com/dynport/gologger"
	"os"
	"strconv"
	"strings"
	"time"
)

var logger = gologger.NewFromEnv()

func Register(router *cli.Router) {
	router.Register("do/droplet/rename", &RenameDroplet{}, "Rename Droplet")
	router.Register("do/droplet/info", &DescribeDroplet{}, "Describe Droplet")
	router.Register("do/droplet/rebuild", &RebuildDroplet{}, "Rebuild droplet")
	router.RegisterFunc("do/droplet/list", ListDropletsAction, "List active droplets")
	router.Register("do/droplet/create", &CreateDroplet{}, "Create new droplet")
	router.Register("do/droplet/destroy", &DestroyDroplet{}, "Destroy Droplet")
	router.RegisterFunc("do/image/list", ListImagesAction, "List available droplet images")
	router.RegisterFunc("do/key/list", ListKeysAction, "List available ssh keys")
	router.RegisterFunc("do/region/list", ListRegionsAction, "List available droplet regions")
	router.RegisterFunc("do/size/list", ListSizesAction, "List available droplet sizes")
}

const (
	DIGITAL_OCEAN_DEFAULT_REGION_ID = 2
	DIGITAL_OCEAN_DEFAULT_SIZE_ID   = 66
	DIGITAL_OCEAN_DEFAULT_IMAGE_ID  = 350076
)

type RenameDroplet struct {
	Id      int    `cli:"type=arg required=true"`
	NewName string `cli:"type=arg required=true"`
}

func ListSizesAction() error {
	logger.Debug("listing sizes")
	account, e := AccountFromEnv()
	if e != nil {
		return e
	}
	logger.Debugf("account is %+v", account)
	table := gocli.NewTable()
	table.Add("Id", "Name")
	sizes, e := account.Sizes()
	if e != nil {
		return e
	}
	for _, size := range sizes {
		table.Add(strconv.Itoa(size.Id), size.Name)
	}
	fmt.Fprintln(os.Stdout, table.String())
	return nil
}

func ListRegionsAction() error {
	logger.Debug("listing regions")
	account, e := AccountFromEnv()
	if e != nil {
		return e
	}
	logger.Debugf("account is %+v", account)
	table := gocli.NewTable()
	table.Add("Id", "Name")
	regions, e := account.Regions()
	if e != nil {
		return e
	}
	for _, region := range regions {
		table.Add(strconv.Itoa(region.Id), region.Name)
	}
	fmt.Fprintln(os.Stdout, table.String())
	return nil
}
func ListKeysAction() error {
	table := gocli.NewTable()
	table.Add("Id", "Name")
	keys, e := CurrentAccount().SshKeys()
	if e != nil {
		return e
	}
	for _, key := range keys {
		table.Add(strconv.Itoa(key.Id), key.Name)
	}
	fmt.Fprintln(os.Stdout, table.String())
	return nil
}

func ListImagesAction() error {
	logger.Debug("listing images")
	logger.Debug("account is %+v", CurrentAccount())
	table := gocli.NewTable()
	table.Add("Id", "Name")
	images, e := account.Images()
	if e != nil {
		return e
	}
	for _, image := range images {
		table.Add(strconv.Itoa(image.Id), image.Name)
	}
	fmt.Fprintln(os.Stdout, table.String())
	return nil
}
func (r *RenameDroplet) Run() error {
	logger.Infof("renaming droplet %d to %s", r.Id, r.NewName)
	_, e := CurrentAccount().RenameDroplet(r.Id, r.NewName)
	if e != nil {
		return e
	}
	logger.Infof("renamed droplet %d to %s", r.Id, r.NewName)
	return nil
}

type DescribeDroplet struct {
	Id int `cli:"type=arg"`
}

func (d *DescribeDroplet) Run() error {
	droplet, e := CurrentAccount().GetDroplet(d.Id)
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Id", fmt.Sprintf("%d", droplet.Id))
	table.Add("Name", droplet.Name)
	table.Add("Status", droplet.Status)
	table.Add("Locked", strconv.FormatBool(droplet.Locked))
	fmt.Println(table)
	return nil
}

var account *digitalocean.Account

func CurrentAccount() *digitalocean.Account {
	if account == nil {
		var e error
		account, e = AccountFromEnv()
		if e != nil {
			logger.Error(e.Error())
			os.Exit(1)
		}
		if account.ImageId == 0 {
			account.ImageId = digitalocean.IMAGE_UBUNTU_13_04_64BIT
		}
		if account.RegionId == 0 {
			account.RegionId = digitalocean.REGION_SF1
		}
		if account.SizeId == 0 {
			account.SizeId = digitalocean.SIZE_512M
		}
		if e != nil {
			ExitWith("unable to load account from env: " + e.Error())
		}
		logger.Debugf("using account %+v", account)
	}
	return account
}

func ExitWith(err interface{}) {
	logger.Error(err)
	os.Exit(1)
}

const (
	ENV_DIGITAL_OCEAN_CLIENT_ID         = "DIGITAL_OCEAN_CLIENT_ID"
	ENV_DIGITAL_OCEAN_API_KEY           = "DIGITAL_OCEAN_API_KEY"
	ENV_DIGITAL_OCEAN_DEFAULT_REGION_ID = "DIGITAL_OCEAN_DEFAULT_REGION_ID"
	ENV_DIGITAL_OCEAN_DEFAULT_SIZE_ID   = "DIGITAL_OCEAN_DEFAULT_SIZE_ID"
	ENV_DIGITAL_OCEAN_DEFAULT_IMAGE_ID  = "DIGITAL_OCEAN_DEFAULT_IMAGE_ID"
	ENV_DIGITAL_OCEAN_DEFAULT_SSH_KEY   = "DIGITAL_OCEAN_DEFAULT_SSH_KEY"
)

func AccountFromEnv() (*digitalocean.Account, error) {
	account := &digitalocean.Account{}
	account.ClientId = os.Getenv(ENV_DIGITAL_OCEAN_CLIENT_ID)
	account.ApiKey = os.Getenv(ENV_DIGITAL_OCEAN_API_KEY)
	account.RegionId, _ = strconv.Atoi(os.Getenv(ENV_DIGITAL_OCEAN_DEFAULT_REGION_ID))
	account.SizeId, _ = strconv.Atoi(os.Getenv(ENV_DIGITAL_OCEAN_DEFAULT_SIZE_ID))
	account.ImageId, _ = strconv.Atoi(os.Getenv(ENV_DIGITAL_OCEAN_DEFAULT_IMAGE_ID))
	account.SshKey, _ = strconv.Atoi(os.Getenv(ENV_DIGITAL_OCEAN_DEFAULT_SSH_KEY))

	allErrors := []string{}

	if account.ClientId == "" {
		allErrors = append(allErrors, fmt.Sprintf("%s must be set in env", ENV_DIGITAL_OCEAN_CLIENT_ID))
	}
	if account.ApiKey == "" {
		allErrors = append(allErrors, fmt.Sprintf("%s must be set in env", ENV_DIGITAL_OCEAN_API_KEY))
	}
	if len(allErrors) > 0 {
		return nil, fmt.Errorf(strings.Join(allErrors, "\n"))
	}
	return account, nil
}

func ListDropletsAction() (e error) {
	logger.Debug("listing droplets")

	droplets, e := CurrentAccount().Droplets()
	if e != nil {
		return e
	}

	if _, e := CurrentAccount().CachedSizes(); e != nil {
		return e
	}

	table := gocli.NewTable()
	if len(droplets) == 0 {
		table.Add("no droplets found")
	} else {
		table.Add("Id", "Created", "Status", "Locked", "Name", "IPAddress", "Region", "Size", "Image")
		for _, droplet := range droplets {
			table.Add(
				strconv.Itoa(droplet.Id),
				droplet.CreatedAt.Format("2006-01-02T15:04"),
				droplet.Status,
				strconv.FormatBool(droplet.Locked),
				droplet.Name,
				droplet.IpAddress,
				fmt.Sprintf("%s (%d)", CurrentAccount().RegionName(droplet.RegionId), droplet.RegionId),
				fmt.Sprintf("%s (%d)", CurrentAccount().SizeName(droplet.SizeId), droplet.SizeId),
				fmt.Sprintf("%s (%d)", CurrentAccount().ImageName(droplet.ImageId), droplet.ImageId),
			)
		}
	}
	fmt.Fprintln(os.Stdout, table.String())
	return nil
}

type CreateDroplet struct {
	Name     string `cli:"type=arg required=true"`
	ImageId  int    `cli:"type=opt short=i required=true"`
	RegionId int    `cli:"type=opt short=r required=true"`
	SizeId   int    `cli:"type=opt short=s required=true"`
	SshKeyId int    `cli:"type=opt short=k"`
}

func (a *CreateDroplet) Run() error {
	started := time.Now()
	droplet := &digitalocean.Droplet{
		Name:     a.Name,
		SizeId:   a.SizeId,
		RegionId: a.RegionId,
		ImageId:  a.ImageId,
		SshKey:   a.SshKeyId,
	}

	droplet, e := CurrentAccount().CreateDroplet(droplet)
	if e != nil {
		return e
	}
	droplet.Account = CurrentAccount()
	logger.Infof("created droplet with id %d", droplet.Id)
	e = digitalocean.WaitForDroplet(droplet)
	logger.Infof("droplet %d ready, ip: %s. total_time: %.1fs", droplet.Id, droplet.IpAddress, time.Now().Sub(started).Seconds())
	return e
}

type DestroyDroplet struct {
	DropletIds []int `cli:"type=arg required=true"`
}

func (a *DestroyDroplet) Run() error {
	logger.Debugf("would destroy droplet with %#v", a.DropletIds)
	for _, id := range a.DropletIds {
		logger.Prefix = fmt.Sprintf("droplet-%d", id)
		droplet, e := CurrentAccount().GetDroplet(id)
		if e != nil {
			logger.Errorf("unable to get droplet for %d", id)
			continue
		}
		logger.Infof("destroying droplet %d", droplet.Id)
		rsp, e := CurrentAccount().DestroyDroplet(droplet.Id)
		if e != nil {
			return e
		}
		logger.Debugf("got response %+v", rsp)
		started := time.Now()
		archived := false
		for i := 0; i < 300; i++ {
			droplet.Reload()
			if droplet.Status == "archive" || droplet.Status == "off" {
				archived = true
				break
			}
			logger.Debug("status " + droplet.Status)
			fmt.Print(".")
			time.Sleep(1 * time.Second)
		}
		fmt.Print("\n")
		logger.Info("droplet destroyed")
		if !archived {
			logger.Errorf("error archiving %d", droplet.Id)
		} else {
			logger.Debugf("archived in %.06f", time.Now().Sub(started).Seconds())
		}
	}
	return nil
}

type RebuildDroplet struct {
	DropletId int `cli:"type=arg required=true"`
	ImageId   int `cli:"type=arg required=true"`
}

func (a *RebuildDroplet) Run() error {
	account := CurrentAccount()
	rsp, e := account.RebuildDroplet(a.DropletId, a.ImageId)
	if e != nil {
		return e
	}
	logger.Debugf("got response %+v", rsp)
	droplet := &digitalocean.Droplet{Id: a.DropletId, Account: account}
	return digitalocean.WaitForDroplet(droplet)
}
