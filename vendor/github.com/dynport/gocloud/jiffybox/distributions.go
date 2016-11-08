package jiffybox

type Distribution struct {
	Key           string
	MinDiskSizeMB int    `json:"minDiskSizeMB"`
	Name          string `json:"name"`
	RootdiskMode  string `json:"rootdiskMode"`
	DefaultKernel string `json:"defaultKernel"`
}

type DistributionsResponse struct {
	DistributionsMap map[string]*Distribution `json:"result"`
}

type DistributionResponse struct {
	Distribution *Distribution `json:"result"`
}

func (rsp *DistributionsResponse) Distributions() (distributions []*Distribution) {
	for key, distribution := range rsp.DistributionsMap {
		distribution.Key = key
		distributions = append(distributions, distribution)
	}
	return distributions
}

func (client *Client) Distributions() (dists []*Distribution, e error) {
	rsp := &DistributionsResponse{}
	e = client.LoadResource("distributions", rsp)
	if e != nil {
		return dists, e
	}
	return rsp.Distributions(), nil
}

func (client *Client) Distribution(id string) (dist *Distribution, e error) {
	rsp := &DistributionResponse{}
	e = client.LoadResource("distribution/"+id, rsp)
	if e != nil {
		return dist, e
	}
	return rsp.Distribution, nil
}
