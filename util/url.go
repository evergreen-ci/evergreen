package util

import (
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

// CheckURL returns errors if str is not in the form of an expected URL
func CheckURL(str string) error {
	u, err := url.ParseRequestURI(str)
	if err != nil {
		return errors.Wrapf(err, "error parsing request uri '%s'", str)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.Errorf("url '%s' scheme '%s' should either be 'http' or 'https'", str, u.Scheme)
	}
	if u.Host == "" {
		return errors.Errorf("url '%s' must have a host name", str)
	}
	// Check to make sure that there is something in the host that could resemble a domain with an extension
	// i.e. "example.com", "sub.example.com", etc.
	domainParts := strings.Split(u.Host, ".")
	if len(domainParts) < 2 {
		return errors.Errorf("url '%s' must have a domain and extension", str)
	}
	return nil
}
