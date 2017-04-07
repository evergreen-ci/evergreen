package util

import (
	"regexp"

	"github.com/pkg/errors"
)

const (
	DefaultSSHPort = "22"
)

// StaticHostInfo stores the connection parameters for connecting to an SSH host.
type StaticHostInfo struct {
	User     string
	Hostname string
	Port     string
}

var userHostPortRegex = regexp.MustCompile(`(?:([\w\-_]+)@)?@?([\w\-_\.]+)(?::(\d+))?`)

// ParseSSHInfo reads in a hostname definition and reads the relevant
// SSH connection information from it. For example,
//  "admin@myhostaddress:24"
// will return
//  StaticHostInfo{User: "admin", Hostname:"myhostaddress", Port: 24}
func ParseSSHInfo(fullHostname string) (*StaticHostInfo, error) {
	matches := userHostPortRegex.FindStringSubmatch(fullHostname)
	if len(matches) == 0 {
		return nil, errors.Errorf("Invalid hostname format: %v", fullHostname)
	} else {
		returnVal := &StaticHostInfo{
			User:     matches[1],
			Hostname: matches[2],
			Port:     matches[3],
		}
		if returnVal.Port == "" {
			returnVal.Port = DefaultSSHPort
		}
		return returnVal, nil
	}
}
