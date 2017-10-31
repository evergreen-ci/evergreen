package upload // import "github.com/nutmegdevelopment/sumologic/upload"

import (
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"time"
	// // 	log "github.com/Sirupsen/logrus"
)

// DebugLogging enables debug logging
func DebugLogging() {
	// // 	log.SetLevel(log.DebugLevel)
}

// GzipThreshold sets the threshold size over which messages
// are compressed when sent.
var GzipThreshold = 2 << 16

// Uploader is an interface to allow easy testing.
type Uploader interface {
	Send([]byte, string) error
}

// httpUploader is a reusable object to upload data to a single
// Sumologic HTTP collector.
type httpUploader struct {
	url       string
	multiline bool
}

// NewUploader creates a new uploader.
func NewUploader(url string) Uploader {
	u := new(httpUploader)
	u.url = url
	return u
}

// Send sends a message to a Sumologic HTTP collector.  It will
// automatically compress messages larger than GzipThreshold.  Optionally,
// a name will be specified, if so this will be added as metadata.
func (u *httpUploader) Send(input []byte, name string) (err error) {
	// nil input is a noop
	if input == nil {
		return
	}

	client := new(http.Client)
	buf := new(bytes.Buffer)

	req := new(http.Request)

	req.Close = true
	client.Timeout = 60 * time.Second

	if len(input) > GzipThreshold {
		// // 	log.Debugf("Data over threshold, compressing (%s bytes)", len(input))
		w := gzip.NewWriter(buf)
		n, err := w.Write(input)
		if err != nil {
			return err
		}
		if n != len(input) {
			return errors.New("Error compressing data")
		}
		err = w.Close()
		if err != nil {
			return err
		}

		req, err = http.NewRequest("POST", u.url, buf)
		if err != nil {
			return err
		}

		req.Header.Set("Content-Encoding", "gzip")
	} else {
		n, err := buf.Write(input)
		if err != nil {
			return err
		}
		if n != len(input) {
			return errors.New("Error sending data")
		}

		req, err = http.NewRequest("POST", u.url, buf)
		if err != nil {
			return err
		}
	}

	if name != "" {
		req.Header.Set("X-Sumo-Name", name)
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	//// 	log.Debugf("Response: %s", resp.Status)

	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}

	return nil
}
