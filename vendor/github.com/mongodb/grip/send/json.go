package send

// NewJSONConsoleLogger builds a Sender instance that prints log
// messages in a JSON formatted to standard output. The JSON formated
// message is taken by calling the Raw() method on the
// message.Composer and Marshalling the results.
func NewJSONConsoleLogger(name string, l LevelInfo) (Sender, error) {
	return setup(MakeJSONConsoleLogger(), name, l)
}

// MakeJSONConsoleLogger returns an un-configured JSON console logging
// instance.
func MakeJSONConsoleLogger() Sender {
	s := MakePlainLogger()
	_ = s.SetFormatter(MakeJSONFormatter())

	return s
}

// NewJSONFileLogger builds a Sender instance that write JSON
// formated log messages to a file, with one-line per message. The
// JSON formated message is taken by calling the Raw() method on the
// message.Composer and Marshalling the results.
func NewJSONFileLogger(name, file string, l LevelInfo) (Sender, error) {
	s, err := MakeJSONFileLogger(file)
	if err != nil {
		return nil, err
	}

	return setup(s, name, l)
}

// MakeJSONFileLogger creates an un-configured JSON logger that writes
// output to the specified file.
func MakeJSONFileLogger(file string) (Sender, error) {
	s, err := MakePlainFileLogger(file)
	if err != nil {
		return nil, err
	}

	if err = s.SetFormatter(MakeJSONFormatter()); err != nil {
		return nil, err
	}

	return s, nil
}
