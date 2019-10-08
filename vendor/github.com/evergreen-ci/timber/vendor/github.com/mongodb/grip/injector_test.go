package grip

import (
	"os"

	"github.com/mongodb/grip/send"
)

func (s *GripSuite) TestSenderGetterReturnsExpectedJournaler() {
	grip := NewJournaler("sender_swap")
	s.Equal(grip.Name(), "sender_swap")

	sender, err := send.NewNativeLogger(grip.Name(), grip.GetSender().Level())
	s.NoError(err)
	s.NoError(grip.SetSender(sender))

	s.Equal(grip.Name(), "sender_swap")
	ns, _ := send.NewNativeLogger("native_sender", s.grip.GetSender().Level())
	defer ns.Close()
	s.IsType(grip.GetSender(), ns)

	sender, err = send.NewFileLogger(grip.Name(), "foo", grip.GetSender().Level())
	s.NoError(grip.SetSender(sender))
	s.NoError(err)

	defer func() { std.Error(os.Remove("foo")) }()

	s.Equal(grip.Name(), "sender_swap")
	s.NotEqual(grip.GetSender(), ns)
	fs, _ := send.NewFileLogger("file_sender", "foo", s.grip.GetSender().Level())
	defer fs.Close()
	s.IsType(grip.GetSender(), fs)

	s.Error(grip.SetSender(nil))

}
