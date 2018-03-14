package notification

import "github.com/mongodb/grip/message"

var targetRegistry = map[string]func() message.Composer{}


