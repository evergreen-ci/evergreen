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

	"github.com/evergreen-ci/evergreen/cli"
	"github.com/jessevdk/go-flags"
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
	opts := &cli.Options{}
	var parser = flags.NewParser(opts, flags.Default)
	_, err := parser.Parse()
	if err != nil {
		panic(err)
	}
	settings, err := cli.LoadSettings(opts)
	if err != nil {
		panic(err)
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
}
