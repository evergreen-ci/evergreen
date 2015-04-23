package hetzner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/dynport/gocli"
	"github.com/dynport/gologger"
)

var logger = gologger.NewFromEnv()

type Server struct {
	ServerIp     string   `json:"server_ip"`
	ServerNumber int      `json:"server_number"`
	ServerName   string   `json:"server_name"`
	Product      string   `json:"product"`
	Dc           string   `json:"dc"`
	Traffic      string   `json:"traffic"`
	Flatrate     bool     `json:"flatrate"`
	Status       string   `json:"status"`
	Throttled    bool     `json:"throttled"`
	Cancelled    bool     `json:"cancelled"`
	PaidUntil    string   `json:"paid_until"`
	Ips          []string `json:"ip"`
	Reset        bool     `json:"reset"`
	Rescue       bool     `json:"rescue"`
	Vnc          bool     `json:"vnc"`
	Windows      bool     `json:"windows"`
	Plesk        bool     `json:"plesk"`
	Cpanel       bool     `json:"cpanel"`
	Wol          bool     `json:"wol"`
}

type Account struct {
	User, Password string
}

func NewFromEnv() *Account {
	return &Account{
		User:     os.Getenv(ENV_USER),
		Password: os.Getenv(ENV_PASSWORD),
	}
}

func (a *Account) Validate() error {
	errors := []string{}
	if a.User == "" {
		errors = append(errors, "User must be set")
	}
	if a.Password == "" {
		errors = append(errors, "Password must be set")
	}
	if len(errors) > 0 {
		return fmt.Errorf(strings.Join(errors, ", "))
	}
	return nil
}

const (
	ENV_USER     = "HETZNER_USER"
	ENV_PASSWORD = "HETZNER_PASSWORD"
)

func AccountFromEnv() (account *Account, e error) {
	user := os.Getenv(ENV_USER)
	password := os.Getenv(ENV_PASSWORD)
	if user == "" || password == "" {
		return nil, fmt.Errorf("%s and %s must be set in env", ENV_USER, ENV_PASSWORD)
	}
	return &Account{User: user, Password: password}, nil
}

func init() {
	if os.Getenv("DEBUG") == "true" {
		logger.LogLevel = gologger.DEBUG
	}
}

func (account *Account) Url() string {
	return fmt.Sprintf("https://%s:%s@robot-ws.your-server.de", account.User, account.Password)
}

func (account *Account) RenameServer(ip string, name string) error {
	values := url.Values{}
	values.Add("server_name", name)
	theUrl := account.Url() + "/server/" + ip
	buf := bytes.Buffer{}
	buf.WriteString(values.Encode())
	logger.Debugf("using values %s", values.Encode())
	req, e := http.NewRequest("POST", theUrl, &buf)
	if e != nil {
		return e
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	rsp, e := loadRequest(req)
	if e != nil {
		return e
	}
	defer rsp.Body.Close()
	if strings.HasPrefix(rsp.Status, "2") {
		logger.Infof("renamed %s to %s", ip, name)
		return nil
	}
	b, e := ioutil.ReadAll(rsp.Body)
	return fmt.Errorf("error renaming server: %s %s", rsp.Status, string(b))
}

func loadRequest(request *http.Request) (rsp *http.Response, e error) {
	logger.Debugf("sending request: METHOD=%s, URL=%s", request.Method, request.URL.String())
	rsp, e = (&http.Client{}).Do(request)
	if e == nil {
		logger.Debugf("got response %s", rsp.Status)
	}
	return rsp, e
}

func (account *Account) Servers() (servers []*Server, e error) {
	req, e := http.NewRequest("GET", account.Url()+"/server", nil)
	if e != nil {
		panic("unable to create request: " + e.Error())
	}
	logger.Debug("fetching servers")
	rsp, e := loadRequest(req)
	if e != nil {
		return servers, e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return servers, e
	}
	if rsp.StatusCode != 200 {
		return nil, fmt.Errorf(string(b))
	}
	st := []map[string]*Server{}
	e = json.Unmarshal(b, &st)
	if e != nil {
		logger.Error(string(b))
		return servers, e
	}
	servers = []*Server{}
	for _, r := range st {
		if server, ok := r["server"]; ok {
			servers = append(servers, server)
		}
	}
	return servers, nil
}

func (server *Server) String() string {
	return fmt.Sprintf("%d: %s (%s)", server.ServerNumber, server.ServerName, server.ServerIp)
}

var router = gocli.NewRouter(nil)
var account *Account

func listServers(args *gocli.Args) error {
	servers, e := account.Servers()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Number", "Name", "Product", "DC", "Ip", "Status")
	for _, server := range servers {
		table.Add(server.ServerNumber, server.ServerName, server.Product, server.Dc, server.ServerIp, server.Status)
	}
	fmt.Println(table)
	return nil
}

func renameServer(args *gocli.Args) error {
	if len(args.Args) != 2 {
		return fmt.Errorf("<ip> <new_name>")
	}
	ip, name := args.Args[0], args.Args[1]
	logger.Infof("renaming servers %s to %s", ip, name)
	return account.RenameServer(ip, name)
}

func loadUrl(url string) (rsp *http.Response, e error) {
	req, e := http.NewRequest("GET", url, nil)
	if e != nil {
		return nil, e
	}
	return loadRequest(req)
}

type ServerResponse struct {
	Server *Server `json:"server"`
}

func (account *Account) LoadServer(ip string) (server *Server, e error) {
	rsp, e := loadUrl(account.Url() + "/server/" + ip)
	if e != nil {
		return nil, e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return nil, e
	}
	serverResponse := &ServerResponse{}
	e = json.Unmarshal(b, serverResponse)
	if e != nil {
		return nil, e
	}
	return serverResponse.Server, e
}
