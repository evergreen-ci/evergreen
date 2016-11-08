package profitbricks

import (
	"encoding/xml"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	ServerId             string              `xml:"serverId"`
	ServerName           string              `xml:"serverName"`
	Cores                int                 `xml:"cores"`
	Ram                  int                 `xml:"ram"`
	InternetAccess       bool                `xml:"internetAccess"`
	ProvisioningState    string              `xml:"provisioningState"`
	Ips                  []string            `xml:"ips"`
	LastModificationTime time.Time           `xml:"lastModificationTime"`
	CreationTime         time.Time           `xml:"creationTime"`
	VirtualMachineState  string              `xml:"virtualMachineState"`
	AvailabilityZone     string              `xml:"availabilityZone"`
	Region               string              `xml:"region"`
	Nics                 []*Nic              `xml:"nics"`
	ConnectedStorages    []*ConnectedStorage `xml:"connectedStorages"`
}

func (server *Server) Lans() string {
	lans := make([]string, 0, len(server.Nics))
	for _, nic := range server.Nics {
		lans = append(lans, strconv.Itoa(nic.LanId))
	}
	return strings.Join(lans, ",")
}

type CreateServerRequest struct {
	XMLName           xml.Name `xml:"request"`
	DataCenterId      string   `xml:"dataCenterId,omitempty"`
	Cores             int      `xml:"cores,omitempty"`
	Ram               int      `xml:"ram,omitempty"`
	ServerName        string   `xml:"serverName,omitempty"`
	BootFromStorageId string   `xml:"bootFromStorageId,omitempty"`
	BootFromImageId   string   `xml:"bootFromImageId,omitempty"`
	InternetAccess    bool     `xml:"internetAccess,omitempty"`
	LanId             int      `xml:"lanId,omitempty"`
	OsType            string   `xml:"osType,omitempty"`
	AvailabilityZone  string   `xml:"availabilityZone,omitempty"`
}

func (client *Client) CreateServer(req *CreateServerRequest) error {
	_, e := client.loadSoapRequest(marshalMultiRequest("createServer", req))
	return e
}

func (client *Client) GetAllServers() (servers []*Server, e error) {
	env, e := client.loadSoapRequest("<tns:getAllServers></tns:getAllServers>")
	if e != nil {
		return nil, e
	}
	return env.Body.GetAllServersResponse, nil
}
