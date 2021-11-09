package awsutil

import "github.com/mongodb/grip/message"

// MakeAPILogMessage creates a message to log information about an API call.
func MakeAPILogMessage(op string, in interface{}) message.Fields {
	return message.Fields{
		"message": "AWS API call",
		"op":      op,
		"input":   in,
	}
}
