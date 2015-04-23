package digitalocean

type Domain struct {
	Name     string `json:"name,omitempty"`
	Ttl      string `json:"ttl,omitempty"`
	ZoneFile string `json:"zone_file,omitempty"`
}

type DomainsResponse struct {
	Domains []*Domain `json:"domain,omitempty"`
	Meta    *Meta     `json:"meta,omitempty"`
}

func (client *Client) Domains() (*DomainsResponse, error) {
	rsp := &DomainsResponse{}
	e := client.loadResponse("/v2/domains", &rsp)
	return rsp, e
}
