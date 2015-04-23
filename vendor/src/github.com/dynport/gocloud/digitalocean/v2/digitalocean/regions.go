package digitalocean

type RegionsResponse struct {
	Regions []*Region `json:"regions,omitempty"`
}

func (client *Client) Regions() (*RegionsResponse, error) {
	rsp := &RegionsResponse{}
	e := client.loadResponse("/v2/regions", rsp)
	return rsp, e
}
