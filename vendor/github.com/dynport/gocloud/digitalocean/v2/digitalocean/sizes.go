package digitalocean

type SizesResponse struct {
	Sizes []*Size `json:"sizes,omitempty"`
}
