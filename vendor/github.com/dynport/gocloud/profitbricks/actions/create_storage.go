package actions

import (
	"fmt"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/profitbricks"
	"github.com/dynport/gologger"
	"os"
)

const (
	CLI_DATACENTER_ID                 = "-d"
	CLI_NAME                          = "-n"
	CLI_SIZE                          = "-s"
	CLI_IMAGE_ID                      = "--image-id"
	CLI_RAM                           = "--ram"
	CLI_CORES                         = "--cores"
	CLI_OS_TYPE                       = "--os-type"
	CLI_INTERNET_ACCESS               = "--public"
	CLI_LAN_ID                        = "--lan-id"
	CLI_ROLLBACK_SNAPSHOT_STORAGE_ID  = "--storage-id"
	CLI_ROLLBACK_SNAPSHOT_SNAPSHOT_ID = "--snapshot-id"
	USAGE_USE_IDS                     = "ID [ID...]"
)

var (
	logger = gologger.NewFromEnv()
)

var defaultDataCenterId = os.Getenv("PROFITBRICKS_DEFAULT_DC_ID")

func dataCenterFlag() *gocli.Flag {
	required := true
	defaultValue := ""
	if defaultDataCenterId != "" {
		required = false
		defaultValue = defaultDataCenterId
	}
	return &gocli.Flag{
		Type:         gocli.STRING,
		Key:          "data_center_id",
		CliFlag:      CLI_DATACENTER_ID,
		Required:     required,
		DefaultValue: defaultValue,
		Description:  "Data Center Id",
	}
}

func init() {
	args := gocli.NewArgs(nil)
	args.RegisterFlag(dataCenterFlag())
	args.RegisterString(CLI_NAME, "name", true, "", "Storage Name")
	args.RegisterInt(CLI_SIZE, "size", true, 0, "Storage Size")
	args.RegisterString(CLI_IMAGE_ID, "image_id", false, "", "Mount Image Id")
	//CreateStorage = &gocli.Action{Handler: CreateStorageHandler, Args: args, Description: "Create Storage"}
}

type CreateServer struct {
	DataCenterId   string `cli:"type=opt short=d required=true"`
	Name           string `cli:"type=arg required=true"`
	Ram            int    `cli:"type=opt short=r default=1024"`
	Cores          int    `cli:"type=opt short=c default=1"`
	OsType         string `cli:"type=opt short=o default=Linux"`
	InternetAccess bool   `cli:"type=opt long=internet-access"`
	LanId          int    `cli:"type=opt short=l default=1"`
	ImageId        string `cli:"type=opt short=i"`
}

func (a *CreateServer) Run() error {
	req := &profitbricks.CreateServerRequest{
		DataCenterId:    a.DataCenterId,
		ServerName:      a.Name,
		Ram:             a.Ram,
		Cores:           a.Cores,
		OsType:          a.OsType,
		InternetAccess:  a.InternetAccess,
		LanId:           a.LanId,
		BootFromImageId: a.ImageId,
	}
	return profitbricks.NewFromEnv().CreateServer(req)
}

func ListAllDataCentersHandler() error {
	dcs, e := profitbricks.NewFromEnv().GetAllDataCenters()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Id", "Name", "Version")
	for _, dc := range dcs {
		table.Add(dc.DataCenterId, dc.DataCenterName, dc.DataCenterVersion)
	}
	fmt.Println(table)
	return nil
}

func ListAllImagesHandler() error {
	client := profitbricks.NewFromEnv()
	images, e := client.GetAllImages()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Id", "Type", "Region", "Name", "Size")
	for _, img := range images {
		table.Add(img.ImageId, img.ImageType, img.Region, img.ImageName, img.ImageSize)
	}
	fmt.Println(table)
	return nil
}

func doSomething(args *gocli.Args, f func(id string) error) error {
	if len(args.Args) == 0 {
		return fmt.Errorf(USAGE_USE_IDS)
	}
	for _, id := range args.Args {
		e := f(id)
		if e != nil {
			return e
		}
	}
	return nil
}

type CreateStorage struct {
	DataCenterId string `cli:"type=opt short=d required=true"`
	Name         string `cli:"type=arg required=true"`
	Size         int    `cli:"type=opt short=s required=true"`
	MountImageId string `cli:"type=opt short=i"`
}

func (a *CreateStorage) Run() error {
	req := &profitbricks.CreateStorageRequest{
		DataCenterId: a.DataCenterId,
		StorageName:  a.Name,
		Size:         a.Size,
		MountImageId: a.MountImageId,
	}
	return profitbricks.NewFromEnv().CreateStorage(req)
}

type DeleteStorage struct {
	Id string `cli:"type=arg required=true"`
}

func (a *DeleteStorage) Run() error {
	return profitbricks.NewFromEnv().DeleteServer(a.Id)
}

func DeleteServerHandler(args *gocli.Args) error {
	return doSomething(args, profitbricks.NewFromEnv().DeleteServer)
}

func StopServerHandler(args *gocli.Args) error {
	return doSomething(args, profitbricks.NewFromEnv().StopServer)
}

func StartServerHandler(args *gocli.Args) error {
	return doSomething(args, profitbricks.NewFromEnv().StartServer)
}

type DeleteServer struct {
	Id string `cli:"type=arg required=true"`
}

func (a *DeleteServer) Run() error {
	return profitbricks.NewFromEnv().DeleteServer(a.Id)
}

type StopServer struct {
	Id string `cli:"type=arg required=true"`
}

func (a *StopServer) Run() error {
	return profitbricks.NewFromEnv().StopServer(a.Id)
}

type StartServer struct {
	Id string `cli:"type=arg required=true"`
}

func (a *StartServer) Run() error {
	return profitbricks.NewFromEnv().StartServer(a.Id)
}

func CreateStorageHandler(args *gocli.Args) error {
	req := &profitbricks.CreateStorageRequest{
		DataCenterId: args.MustGetString(CLI_DATACENTER_ID),
		StorageName:  args.MustGetString(CLI_NAME),
		Size:         args.MustGetInt(CLI_SIZE),
		MountImageId: args.MustGetString(CLI_IMAGE_ID),
	}
	logger.Infof("creating storage with %#v", req)
	return profitbricks.NewFromEnv().CreateStorage(req)
}
