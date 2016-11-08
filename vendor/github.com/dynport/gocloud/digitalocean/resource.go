package digitalocean

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"
)

type resource struct {
	Url      string
	CachedAt time.Time
	Content  []byte
}

func (r *resource) load(opts *fetchOptions) (e error) {
	if opts != nil {
		if e := r.loadCached(opts); e == nil {
			logger.Debugf("loaded cached resource (expires in %s)", r.CachedAt.Add(opts.ttl).Sub(time.Now()).String())
			return nil
		} else {
			logger.Debugf(e.Error())
		}
	}
	rsp, e := http.Get(r.Url)
	if e != nil {
		return e
	}
	defer rsp.Body.Close()
	r.Content, e = ioutil.ReadAll(rsp.Body)
	if e != nil {
		return e
	}
	if opts != nil {
		if e := r.store(); e != nil {
			logger.Error(e.Error())
		}
	}
	return nil
}

func (r *resource) store() error {
	r.CachedAt = time.Now()
	p := r.path()
	os.Remove(p)
	if e := os.MkdirAll(path.Dir(p), 0755); e != nil {
		return e
	}
	f, e := os.Create(p)
	if e != nil {
		return e
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(r)
}

func (r *resource) path() string {
	hash := md5.New()
	hash.Write([]byte(r.Url))
	sum := fmt.Sprintf("%x", hash.Sum(nil))
	return cachedPath + "/" + sum + ".json"
}

func (r *resource) loadCached(opts *fetchOptions) error {
	p := r.path()
	stat, e := os.Stat(p)
	if e != nil {
		return e
	}
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	e = json.NewDecoder(f).Decode(r)
	if e != nil {
		return e
	}
	logger.Debugf("%v %v", r.CachedAt, opts.ttl.String())
	if time.Now().After(stat.ModTime().Add(opts.ttl)) {
		logger.Debug("resource expired")
		return fmt.Errorf("expired")
	}
	return nil
}
