package send

// SenderType provides an indicator of what kind of sender is
// currently in use. In practice Sender constructors return an object
// of the Sender interface, with the actual implementations private.
type SenderType int

// Sender types provide identifiers for the built-in sender
// implementation provided by the send package. These values are
// returned by the Sender's Type() method.
const (
	Custom SenderType = iota
	Systemd
	Native
	JSON
	Syslog
	Internal
	Multi
	File
	Slack
	XMPP
	Stream
	Bootstrap
	Buildlogger
)

func (t SenderType) String() string {
	switch t {
	case Systemd:
		return "systemd"
	case Native:
		return "native"
	case Syslog:
		return "syslog"
	case Internal:
		return "internal"
	case File:
		return "file"
	case Bootstrap:
		return "bootstrap"
	case Buildlogger:
		return "buildlogger"
	case Custom:
		return "custom"
	case Slack:
		return "slack"
	case XMPP:
		return "xmpp"
	case JSON:
		return "json"
	case Stream:
		return "stream"
	case Multi:
		return "multi"
	default:
		return "native"
	}
}
