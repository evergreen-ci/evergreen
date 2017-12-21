// This program uses the Evergreen REST API to grab the most recent configuration
// for each project and write each configuration file to the file system for
// further analysis. This program's purpose is to provide a useful utility that's
// too niche for the main CLI tool and provide a proof of concept to the REST API.
//
// Usage: upon execution, this program will load the user's CLI settings
//   (default `~/.evergreen.yml`) and use them to download all project files
//   to the working directory in the form of "<project_id>.yml"
//
// -c / --config can pass in a different CLI settings file
//
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/evergreen-ci/evergreen/operations"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/mongodb/grip"
	"github.com/urfave/cli"
)

// uiClient handles requests against the UI server.
type uiClient struct {
	client *http.Client
	user   string
	key    string
	uiRoot string
}

// get executes a request against the given REST route, panicking on any errors.
func (c *uiClient) get(route string) *http.Response {
	url := c.uiRoot + "/rest/v1/" + route
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Add("Auth-Username", c.user)
	req.Header.Add("Api-Key", c.key)
	resp, err := c.client.Do(req)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("Bad Status (%v) : %v", url, resp.StatusCode))
	}
	return resp
}

// mustDecodeJSON is a quick helper for panicking if a Reader cannot be read as JSON.
func mustDecodeJSON(r io.Reader, i interface{}) {
	err := json.NewDecoder(r).Decode(i)
	if err != nil {
		panic(err)
	}
}

// fetchAndWriteConfig downloads the most recent config for a project
// and writes it to "project_name.yml" locally.
func fetchAndWriteConfig(uic *uiClient, project string) {
	fmt.Println("Downloading configuration for", project)
	resp := uic.get("projects/" + project + "/versions")
	v := struct {
		Versions []struct {
			Id string `json:"version_id"`
		} `json:"versions"`
	}{}
	mustDecodeJSON(resp.Body, &v)
	if len(v.Versions) == 0 {
		fmt.Println("WARNING:", project, "has no versions")
		return
	}
	resp = uic.get("versions/" + v.Versions[0].Id + "?config=1")
	c := struct {
		Config string `json:"config"`
	}{}
	mustDecodeJSON(resp.Body, &c)
	if len(c.Config) == 0 {
		fmt.Println("WARNING:", project, "has empty config file")
		return
	}
	err := ioutil.WriteFile(project+".yml", []byte(c.Config), 0666)
	if err != nil {
		panic(err)
	}
}

func main() {
	app := cli.NewApp()
	app.Name = "all_configs"
	app.Usage = "Evergreen Configuration Download Tool"

	userHome, err := homedir.Dir()
	if err != nil {
		// workaround for cygwin if we're on windows but couldn't get a homedir
		if runtime.GOOS == "windows" && len(os.Getenv("HOME")) > 0 {
			userHome = os.Getenv("HOME")
		}
	}
	confPath := filepath.Join(userHome, ".evergreen.yml")

	// These are global options. Use this to configure logging or
	// other options independent from specific sub commands.
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "conf, config, c",
			Usage: "specify the path for the evergreen CLI config",
			Value: confPath,
		},
	}
	app.Action = func(c *cli.Context) error {
		settings, err := operations.NewClientSetttings(c.GlobalString("config"))
		if err != nil {
			return err
		}

		uic := &uiClient{
			client: &http.Client{},
			user:   settings.User,
			key:    settings.APIKey,
			uiRoot: settings.UIServerHost,
		}

		// fetch list of all projects
		resp := uic.get("projects")
		pList := struct {
			Projects []string `json:"projects"`
		}{}
		mustDecodeJSON(resp.Body, &pList)

		for _, p := range pList.Projects {
			fetchAndWriteConfig(uic, p)
		}
		return nil
	}

	grip.CatchEmergencyFatal(app.Run(os.Args))
}
