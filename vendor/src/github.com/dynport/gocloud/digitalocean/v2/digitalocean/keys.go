package digitalocean

type KeysResponse struct {
	SshKeys []*SshKey `json:"ssh_keys"`
}

type SshKey struct {
	Id          int    `json:"id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	PublicKey   string `json:"public_key,omitempty"`
	Name        string `json:"name,omitempty"`
}

func (client *Client) Keys() (*KeysResponse, error) {
	rsp := &KeysResponse{}
	e := client.loadResponse("/v2/account/keys", rsp)
	return rsp, e
}
