package logging

import (
	"fmt"

	"github.com/mongodb/grip/level"
)

func (s *GripInternalSuite) TestPriorityConverter() {
	inputs := map[string][]interface{}{
		"info":      []interface{}{40, level.Info},
		"debug":     []interface{}{30, level.Debug},
		"trace":     []interface{}{20, level.Trace},
		"emergency": []interface{}{100, level.Emergency},
		"alert":     []interface{}{90, level.Alert},
		"critical":  []interface{}{80, level.Critical},
		"error":     []interface{}{70, level.Error},
		"warning":   []interface{}{60, level.Warning},
		"notice":    []interface{}{50, level.Notice},
	}

	for name, values := range inputs {
		if !s.Len(values, 2) {
			continue
		}
		fmt.Printf("%T: %+v (%s)\n", values, values, name)

		out := convertPriority(name, -1)
		s.Equal(name, out.String())
		s.Equal(values[0], int(out))

		s.NoError(s.grip.SetDefaultLevel(name))
		s.Equal(name, s.grip.DefaultLevel().String())
		s.Equal(out, s.grip.DefaultLevel())

		for _, p := range values {
			out = convertPriority(p, -1)
			s.Equal(name, out.String())

			s.NoError(s.grip.SetDefaultLevel(name))
			s.Equal(name, s.grip.DefaultLevel().String())
			s.Equal(out, s.grip.DefaultLevel())

			switch p := p.(type) {
			case int:
				s.Equal(p, int(out), fmt.Sprintf("%T", p))
			case level.Priority:
				s.Equal(p, out, fmt.Sprintf("%T", p))
			default:
				panic("unreachable")
			}
		}
	}

	invalidCases := []interface{}{
		"invalid", "non", true, []string{"foo", "bar"}, nil,
		101, 1000, 0, -1, -100, -1000,
	}
	s.NoError(s.grip.SetDefaultLevel(level.Info))
	s.Equal(level.Info, s.grip.DefaultLevel())

	for _, p := range invalidCases {
		out := convertPriority(p, level.Notice)
		s.NoError(s.grip.SetDefaultLevel(p))
		s.Equal("notice", out.String())
		s.Equal(level.Notice, out)

		s.Equal(s.grip.DefaultLevel(), level.Info)
	}

}
