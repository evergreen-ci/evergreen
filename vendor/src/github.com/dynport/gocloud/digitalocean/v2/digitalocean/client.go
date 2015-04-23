package digitalocean

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

type DropletsList struct {
}

type Client struct {
	*http.Client
}

var root = "https://api.digitalocean.com"

func (c *Client) loadResponse(path string, i interface{}) error {
	rsp, e := c.Get(root + "/" + strings.TrimPrefix(path, "/"))
	if e != nil {
		return e
	}
	defer rsp.Body.Close()
	b, e := ioutil.ReadAll(rsp.Body)
	if e != nil {
		return e
	}
	if rsp.Status[0] != '2' {
		return fmt.Errorf("expected status 2xx, got %s: %s", rsp.Status, string(b))
	}
	dbg.Printf("%s", string(b))
	return json.Unmarshal(b, &i)
}

func New(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("token must be set")
	}
	return &Client{Client: &http.Client{Transport: &transport{apiToken: token}}}, nil
}

func NewFromEnv() (*Client, error) {
	token := os.Getenv("DIGITAL_OCEAN_API_KEY")
	if token == "" {
		return nil, fmt.Errorf("DIGITAL_OCEAN_API_KEY must be set in env")
	}
	return New(token)
}

type transport struct {
	apiToken string
}

func (c *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Bearer "+c.apiToken)
	return http.DefaultClient.Do(r)
}
