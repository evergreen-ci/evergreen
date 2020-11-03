package util

import (
	"net/http"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// SpotHostWillTerminateSoon returns true if the EC2 spot host it is running on will terminate soon.
func SpotHostWillTerminateSoon() bool {
	const url = "http://169.254.169.254/latest/meta-data/spot/termination-time"
	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)
	resp, err := c.Get(url)
	if err != nil {
		grip.Info(errors.Wrap(err, "problem getting termination endpoint"))
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true
	}
	return false
}
