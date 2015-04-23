package digitalocean

type Size struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type SizeResponse struct {
	Status string  `json:"status"`
	Sizes  []*Size `json:"sizes"`
}
