package s3

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
)

type ListBucketOptions struct {
	Marker string
	Prefix string
}

func (client *Client) ListBucket(bucket string) (r *ListBucketResult, e error) {
	return client.ListBucketWithOptions(bucket, nil)
}

func (client *Client) ListBucketWithOptions(bucket string, opts *ListBucketOptions) (r *ListBucketResult, e error) {
	u := "https://" + client.EndpointHost() + "/" + bucket
	if opts != nil {
		v := &url.Values{}
		if opts.Marker != "" {
			v.Add("marker", opts.Marker)
		}
		if opts.Prefix != "" {
			v.Add("prefix", opts.Prefix)
		}
		if len(*v) > 0 {
			u += "?" + v.Encode()
		}
	}
	req, e := http.NewRequest("GET", u, nil)
	if e != nil {
		return r, e
	}
	req.Header.Add("Host", bucket+"."+client.EndpointHost())
	_, b, e := client.signAndDoRequest(bucket, req)
	if e != nil {
		return nil, e
	}
	r = &ListBucketResult{}
	e = xml.Unmarshal(b, r)
	if e != nil {
		return r, fmt.Errorf("ERROR: %s (%s)", e.Error(), string(b))
	}
	return r, e
}
