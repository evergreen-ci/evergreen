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
		return errors.Wrapf(err, "parsing request URI '%s'", str)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.Errorf("scheme '%s' for URL '%s' should either be 'http' or 'https'", u.Scheme, str)
	}
	if u.Host == "" {
		return errors.Errorf("URL '%s' must have a host name", str)
	}
	// Check to make sure that there is something in the host that could resemble a domain with an extension
	// i.e. "example.com", "sub.example.com", etc.
	domainParts := strings.Split(u.Host, ".")
	if len(domainParts) < 2 {
		return errors.Errorf("URL '%s' must have a domain and extension", str)
	}
	return nil
}

// HttpsUrl appends "https://" to the given URL string.
func HttpsUrl(url string) string {
	return "https://" + url
}
