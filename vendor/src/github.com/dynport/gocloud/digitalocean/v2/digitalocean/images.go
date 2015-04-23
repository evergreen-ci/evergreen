package digitalocean

import "fmt"

type ImagesResponse struct {
	Images []*Image `json:"images,omitempty"`
	Meta   *Meta    `json:"meta,omitempty"`
}

type ImageResponse struct {
	Image *Image `json:"image,omitempty"`
}

func (client *Client) Images(page int) (*ImagesResponse, error) {
	rsp := &ImagesResponse{}
	p := "/v2/images"
	if page > 1 {
		p += fmt.Sprintf("?page=%d", page)
	}
	e := client.loadResponse(p, rsp)
	return rsp, e
}

func (client *Client) Image(idOrSlug string) (*ImageResponse, error) {
	rsp := &ImageResponse{}
	e := client.loadResponse("/v2/images/"+idOrSlug, rsp)
	return rsp, e
}
