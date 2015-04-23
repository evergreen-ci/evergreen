package digitalocean

import (
	"os"
	"strconv"
	"time"
)

type Image struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	Distribution string `json:"distribution"`
}

type ImagesReponse struct {
	Status string   `json:"status"`
	Images []*Image `json:"images"`
}

type ImageResponse struct {
	Status string `json:"status"`
	Image  *Image `json:"image"`
}

func (self *Account) Images() (images []*Image, e error) {
	imagesReponse := &ImagesReponse{}
	e = self.loadResource("/images", imagesReponse, cacheFor(24*time.Hour))
	if e != nil {
		return nil, e
	}
	images = imagesReponse.Images
	return images, nil
}

var cachedPath = os.Getenv("HOME") + "/.gocloud/cache/digitalocean"

func (self *Account) GetImage(id int) (image *Image, e error) {
	imageReponse := &ImageResponse{}
	e = self.loadResource("/images/"+strconv.Itoa(id), imageReponse, cacheFor(24*time.Hour))
	image = imageReponse.Image
	return
}
