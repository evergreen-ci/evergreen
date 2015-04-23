package util

import "regexp"
import "fmt"

const (
	DefaultSSHPort = "22"
)

type StaticHostInfo struct {
	User     string
	Hostname string
	Port     string
}

var USER_HOST_PORT_REGEX = regexp.MustCompile("(?:([\\w\\-_]+)@)?@?([\\w\\-_\\.]+)(?::(\\d+))?")

func ParseSSHInfo(fullHostname string) (*StaticHostInfo, error) {
	matches := USER_HOST_PORT_REGEX.FindStringSubmatch(fullHostname)
	if len(matches) == 0 {
		return nil, fmt.Errorf("Invalid hostname format: %v", fullHostname)
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
