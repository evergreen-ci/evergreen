package profitbricks

import (
	"encoding/xml"
	"fmt"
	"github.com/dynport/gologger"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
)

var logger = gologger.NewFromEnv()

const (
	PROFITBRICKS_USER        = "PROFITBRICKS_USER"
	PROFITBRICKS_PASSWORD    = "PROFITBRICKS_PASSWORD"
	GetAllDataCentersRequest = `<tns:getAllDataCenters></tns:getAllDataCenters>`
)

func NewFromEnv() *Client {
	client := &Client{
		User:     os.Getenv(PROFITBRICKS_USER),
		Password: os.Getenv(PROFITBRICKS_PASSWORD),
	}
	if client.User == "" || client.Password == "" {
		fmt.Printf("ERROR: %s and %s must be set\n", PROFITBRICKS_USER, PROFITBRICKS_PASSWORD)
		os.Exit(1)
	}
	return client
}

type Client struct {
	User     string
	Password string
}

type GetDataCenterResponse struct {
	*DataCenter
}

type GetAllDataCentersResponse struct {
	*DataCenter
}

type GetAllSnapshotsResponse struct {
	*DataCenter
}

func (client *Client) GetAllSnapshots() (snapshots []*Snapshot, e error) {
	env, e := client.loadSoapRequest("<tns:getAllSnapshots></tns:getAllSnapshots>")
	if e != nil {
		return nil, e
	}
	return env.Body.GetAllSnapshotsResponse, nil
}

func (client *Client) GetAllStorages() (storages []*Storage, e error) {
	env, e := client.loadSoapRequest("<tns:getAllStorages></tns:getAllStorages>")
	if e != nil {
		return nil, e
	}
	return env.Body.GetAllStoragesResponse, nil
}

func (client *Client) DeleteStorage(id string) error {
	_, e := client.loadSoapRequest("<tns:deleteStorage><storageId>" + id + "</storageId></tns:deleteStorage>")
	return e
}

func (client *Client) CreateStorage(req *CreateStorageRequest) error {
	_, e := client.loadSoapRequest(marshalMultiRequest("createStorage", req))
	return e
}

func (client *Client) sendServerAction(action string, id string) error {
	_, e := client.loadSoapRequest("<tns:" + action + "><serverId>" + id + "</serverId></tns:" + action + ">")
	return e
}

type StartServerRequest struct {
	DataCenterId      string
	Cores             int
	Ram               int
	ServerName        string
	BootFromStorageId string
	BootFromImageId   string
	InternetAccess    string
	LanId             int
	OsType            string
	AvailabilityZone  string
}

func (client *Client) DeleteServer(id string) error {
	return client.sendServerAction("deleteServer", id)
}

func (client *Client) ResetServer(id string) error {
	return client.sendServerAction("resetServer", id)
}

func (client *Client) StopServer(id string) error {
	return client.sendServerAction("stopServer", id)
}

func (client *Client) StartServer(id string) error {
	return client.sendServerAction("startServer", id)
}

func (client *Client) GetDataCenter(id string) (dc *DataCenter, e error) {
	env, e := client.loadSoapRequest("<tns:getDataCenter><dataCenterId>" + id + "</dataCenterId></tns:getDataCenter>")
	if e != nil {
		return nil, e
	}
	return env.Body.GetDataCenterResponse.DataCenter, nil
}

func (client *Client) loadSoapRequest(body string) (env *Envelope, e error) {
	reader := strings.NewReader(SOAP_HEADER + body + SOAP_FOOTER)
	logger.Debugf("sending body %s", body)
	req, e := http.NewRequest("POST", "https://"+client.User+":"+client.Password+"@api.profitbricks.com/1.2", reader)
	if e != nil {
		return nil, e
	}
	req.Header.Add("Content-Type", "text/xml;charset=UTF-8")
	started := time.Now()
	rsp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	logger.Debugf("got response in %.06f", time.Now().Sub(started).Seconds())
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return nil, e
	}
	logger.Debugf("got status %s, response %s", rsp.Status, string(b))
	if rsp.Status[0] != '2' {
		return nil, fmt.Errorf("ERROR: %s => %s", rsp.Status, string(b))
	}
	logger.Debugf("got response %q", string(b))
	env = &Envelope{}
	e = xml.Unmarshal(b, env)
	if e != nil {
		return nil, e
	}
	return env, nil
}

func (client *Client) GetAllImages() (images []*Image, e error) {
	env, e := client.loadSoapRequest("<tns:getAllImages></tns:getAllImages>")
	if e != nil {
		return nil, e
	}
	return env.Body.GetAllImagesResponse, nil
}

func (client *Client) GetAllDataCenters() (dcs []*DataCenter, e error) {
	env, e := client.loadSoapRequest(GetAllDataCentersRequest)
	if e != nil {
		return nil, e
	}
	return env.Body.GetAllDataCenters, nil
}
