package send

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

type buildlogger struct {
	conf   *BuildloggerConfig
	name   string
	cache  chan []interface{}
	client *http.Client
	*Base
}

// BuildloggerConfig describes the configuration needed for a Sender
// instance that posts log messages to a buildlogger service
// (e.g. logkeeper.)
type BuildloggerConfig struct {
	// CreateTest controls
	CreateTest bool
	URL        string

	// The following values are used by the buildlogger service to
	// attach metadata to the logs. The GetBuildloggerConfig
	// method populates Number, Phase, Builder, and Test from
	// environment variables, though you can set them directly in
	// your application. You must set the Command value directly.
	Number  int
	Phase   string
	Builder string
	Test    string
	Command string

	// Configure a local sender for "fallback" operations and to
	// collect the location (URLS) of the buildlogger output
	Local Sender

	buildID  string
	testID   string
	username string
	password string
}

// ReadCredentialsFromFile parses a JSON file for buildlogger
// credentials and updates the config.
func (c *BuildloggerConfig) ReadCredentialsFromFile(fn string) error {
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return errors.New("credentials file does not exist")
	}

	contents, err := ioutil.ReadFile(fn)
	if err != nil {
		return err
	}

	out := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}
	if err := json.Unmarshal(contents, &out); err != nil {
		return err
	}

	c.username = out.Username
	c.password = out.Password

	return nil
}

// SetCredentials configures the username and password of the
// BuildLoggerConfig object. Use to programatically update the
// credentials in a configuration object instead of reading from the
// credentials file with ReadCredentialsFromFile(),
func (c *BuildloggerConfig) SetCredentials(username, password string) {
	c.username = username
	c.password = password
}

// GetBuildloggerConfig produces a BuildloggerConfig object, reading
// default values from environment variables, although you can set
// these options yourself.
//
// You must also populate the credentials seperatly using either the
// ReadCredentialsFromFile or SetCredentials methods. If the
// BUILDLOGGER_CREDENTIALS environment variable is set,
// GetBuildloggerConfig will read credentials from this file.
//
// Buildlogger has a concept of a build, with a global log, as well as
// subsidiary "test" logs. To exercise this functionality, you will
// use a single Buildlogger config instance to create individual
// Sender instances that target each of these output formats.
//
// The Buildlogger config has a Local attribute which is used by the
// logger config when: the Buildlogger instance cannot contact the
// messages sent to the buildlogger. Additional the Local Sender also
// logs the remote location of the build logs. This Sender
// implementation is used in the default ErrorHandler for this
// implementation.
//
// Create a BuildloggerConfig instance, set up the crednetials if
// needed, and create a sender. This will be the "global" log, in
// buildlogger terminology. Then, set set the CreateTest attribute,
// and generate additional per-test senders. For example:
//
//    conf := GetBuildloggerConfig()
//    global := MakeBuildlogger("<name>-global", conf)
//    // ... use global
//    conf.CreateTest = true
//    testOne := MakeBuildlogger("<name>-testOne", conf)
func GetBuildloggerConfig() (*BuildloggerConfig, error) {
	conf := &BuildloggerConfig{
		URL:     os.Getenv("BULDLOGGER_URL"),
		Phase:   os.Getenv("MONGO_PHASE"),
		Builder: os.Getenv("MONGO_BUILDER_NAME"),
		Test:    os.Getenv("MONGO_TEST_FILENAME"),
	}

	if creds := os.Getenv("BUILDLOGGER_CREDENTIALS"); creds != "" {
		if err := conf.ReadCredentialsFromFile(creds); err != nil {
			return nil, err
		}
	}

	buildNum, err := strconv.Atoi(os.Getenv("MONGO_BUILD_NUMBER"))
	if err != nil {
		return nil, err
	}
	conf.Number = buildNum

	if conf.Test == "" {
		conf.Test = "unknown"
	}

	if conf.Phase == "" {
		conf.Phase = "unknown"
	}

	return conf, nil
}

// GetGlobalLogURL returns the URL for the current global log in use.
// Must use after constructing the buildlogger instance.
func (c *BuildloggerConfig) GetGlobalLogURL() string {
	return fmt.Sprintf("%s/build/%s", c.URL, c.buildID)
}

// GetTestLogURL returns the current URL for the test log currently in
// use. Must use after constructing the buildlogger instance.
func (c *BuildloggerConfig) GetTestLogURL() string {
	return fmt.Sprintf("%s/build/%s/test/%s", c.URL, c.buildID, c.testID)
}

// NewBuildlogger constructs a Buildlogger-targeted Sender, with level
// information set. See MakeBuildlogger and GetBuildloggerConfig for
// more information.
func NewBuildlogger(name string, conf *BuildloggerConfig, l LevelInfo) (Sender, error) {
	s, err := MakeBuildlogger(name, conf)
	if err != nil {
		return nil, err
	}

	return setup(s, name, l)
}

// MakeBuildlogger constructs a buildlogger targeting sender using the
// BuildloggerConfig object for configuration. Generally you will
// create a "global" instance, and then several subsidiary Senders
// that target specific tests. See the documentation of
// GetBuildloggerConfig for more information.
//
// Upon creating a logger, this method will write, to standard out,
// the URL that you can use to view the logs produced by this Sender.
func MakeBuildlogger(name string, conf *BuildloggerConfig) (Sender, error) {
	b := &buildlogger{
		name:   name,
		conf:   conf,
		cache:  make(chan []interface{}),
		client: &http.Client{Timeout: 10 * time.Second},
		Base:   NewBase(name),
	}

	if b.conf.Local == nil {
		b.conf.Local = MakeNative()
	}

	if err := b.SetErrorHandler(ErrorHandlerFromSender(b.conf.Local)); err != nil {
		return nil, err
	}

	if b.conf.buildID == "" {
		data := struct {
			Builder string `json:"builder"`
			Number  int    `json:"buildnum"`
		}{
			Builder: name,
			Number:  conf.Number,
		}

		out, err := b.doPost(data)
		if err != nil {
			b.conf.Local.Send(message.NewErrorMessage(level.Error, err))
			return nil, err
		}

		b.conf.buildID = out.ID

		b.conf.Local.Send(message.NewLineMessage(level.Notice,
			"Writing logs to buildlogger global log at:",
			b.conf.GetGlobalLogURL()))
	}

	if b.conf.CreateTest {
		data := struct {
			Filename string `json:"test_filename"`
			Command  string `json:"command"`
			Phase    string `json:"phase"`
		}{
			Filename: conf.Test,
			Command:  conf.Command,
			Phase:    conf.Phase,
		}

		out, err := b.doPost(data)
		if err != nil {
			b.conf.Local.Send(message.NewErrorMessage(level.Error, err))
			return nil, err
		}

		b.conf.testID = out.ID

		b.conf.Local.Send(message.NewLineMessage(level.Notice,
			"Writing logs to buildlogger test log at:",
			b.conf.GetTestLogURL()))
	}

	return b, nil
}

func (b *buildlogger) Send(m message.Composer) {
	if b.Level().ShouldLog(m) {
		req := [][]interface{}{
			[]interface{}{float64(time.Now().Unix()), m.String()},
		}

		out, err := json.Marshal(req)
		if err != nil {
			b.conf.Local.Send(message.NewErrorMessage(level.Error, err))
			return
		}

		if err := b.postLines(bytes.NewBuffer(out)); err != nil {
			b.ErrorHandler(err, message.NewBytesMessage(b.level.Default, out))
		}
	}
}

func (b *buildlogger) SetName(n string) {
	b.conf.Local.SetName(n)
	b.Base.SetName(n)
}

func (b *buildlogger) SetLevel(l LevelInfo) error {
	if err := b.Base.SetLevel(l); err != nil {
		return err
	}

	if err := b.conf.Local.SetLevel(l); err != nil {
		return err
	}

	return nil
}

///////////////////////////////////////////////////////////////////////////
//
// internal methods and helpers
//
///////////////////////////////////////////////////////////////////////////

type buildLoggerIDResponse struct {
	ID string `json:"id"`
}

func (b *buildlogger) doPost(data interface{}) (*buildLoggerIDResponse, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", b.getURL(), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.SetBasicAuth(b.conf.username, b.conf.password)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)

	out := &buildLoggerIDResponse{}
	if err := decoder.Decode(out); err != nil {
		return nil, err
	}

	return out, nil
}

func (b *buildlogger) getURL() string {
	parts := []string{b.conf.URL, "build"}

	if b.conf.buildID != "" {
		parts = append(parts, b.conf.buildID)
	}

	// if we want to create a test id, (e.g. the CreateTest flag
	// is set and we don't have a testID), then the following URL
	// will generate a testID.
	if b.conf.CreateTest && b.conf.testID == "" {
		// this will create the testID.
		parts = append(parts, "test")
	}

	// if a test id is present, then we want to append to the test logs.
	if b.conf.testID != "" {
		parts = append(parts, "test", b.conf.testID)
	}

	return strings.Join(parts, "/")
}

func (b *buildlogger) postLines(body io.Reader) error {
	req, err := http.NewRequest("POST", b.getURL(), body)

	if err != nil {
		return err
	}
	req.SetBasicAuth(b.conf.username, b.conf.password)

	_, err = b.client.Do(req)
	return err
}
