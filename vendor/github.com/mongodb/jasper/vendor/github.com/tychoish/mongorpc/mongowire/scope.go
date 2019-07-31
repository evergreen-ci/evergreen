package mongowire

import (
	"github.com/pkg/errors"
)

type OpScope struct {
	Type    OpType
	Context string
	Command string
}

func (s *OpScope) Validate() error {
	switch s.Type {
	case OP_COMMAND:
		if s.Command == "" {
			return errors.New("commands much identify a named command")
		}

		if s.Context == "" {
			return errors.New("command ops must specify a scope (dbname only)")
		}

		return nil
	case OP_DELETE:
		if s.Context == "" {
			return errors.New("delete ops must specify a scope (dbname.collection)")
		}

		if s.Command != "" {
			return errors.New("kill cursors cannot specify a command name")
		}

		return nil
	case OP_UPDATE:
		if s.Context == "" {
			return errors.New("update ops must specify a scope (dbname.collection)")
		}

		if s.Command != "" {
			return errors.New("updates cannot specify a command name")
		}

		return nil
	case OP_KILL_CURSORS:
		if s.Context != "" {
			return errors.New("kill cursors cannot specify a scope")
		}

		if s.Command != "" {
			return errors.New("kill cursors cannot specify a command name")
		}

		return nil
	case OP_QUERY:
		if s.Context == "" {
			return errors.New("query ops must specify a scope (dbname.collection)")
		}

		if s.Command != "" {
			return errors.New("query ops cannot specify a command name")
		}

		return nil
	case OP_INSERT:
		if s.Context == "" {
			return errors.New("insert ops must specify a scope (dbname.collection)")
		}

		if s.Command != "" {
			return errors.New("insert ops cannot specify a command name")
		}

		return nil
	case OP_GET_MORE:
		if s.Context == "" {
			return errors.New("get more ops must specify a scope (dbname.collection)")
		}

		if s.Command != "" {
			return errors.New("get more ops cannot specify a command name")
		}

		return nil
	default:
		return errors.New("must specify a valid request op type")
	}
}
