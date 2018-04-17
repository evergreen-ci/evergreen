package evergreen

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	legacyDB "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
)

var globalEnvState *envState

func init() {
	ResetEnvironment()
}

// GetEnvironment returns the global application level
// environment. This implementation is thread safe, but must be
// configured before use.
//
// In general you should call this operation once per process
// execution and pass the Environment interface through your
// application like a context, although there are cases in legacy code
// (e.g. models) and in the implementation of amboy jobs where it is
// necessary to access the global environment. There is a mock
// implementation for use in testing.
func GetEnvironment() Environment { return globalEnvState }

func ResetEnvironment() { globalEnvState = &envState{} }

// Environment provides application-level services (e.g. databases,
// configuration, queues.
type Environment interface {
	// Configure initializes the object. Some implementations may
	// not allow the same instance to be configured more than
	// once.
	//
	// If Configure returns without an error, you should assume
	// that the queues have been started, there was no issue
	// establishing a connection to the database, and that the
	// local and remote queues have started.
	Configure(context.Context, string, *DBSettings) error

	// Returns the settings object. The settings object is not
	// necessarily safe for concurrent access.
	Settings() *Settings
	Session() db.Session

	// The Environment provides access to two queue's, a
	// local-process level queue that is not persisted between
	// runs, and a remote shared queue that all processes can use
	// to distribute work amongst the application tier.
	//
	// The LocalQueue is not durable, and results aren't available
	// between process restarts.
	LocalQueue() amboy.Queue
	RemoteQueue() amboy.Queue

	// ClientConfig provides access to a list of the latest evergreen
	// clients, that this server can serve to users
	ClientConfig() *ClientConfig

	// GetSender provides a grip Sender configured with the environment's
	// settings. These Grip senders must be used with Composers that specify
	// all message details.
	GetSender(SenderKey) (send.Sender, error)
}

type envState struct {
	remoteQueue  amboy.Queue
	localQueue   amboy.Queue
	settings     *Settings
	session      *mgo.Session
	mu           sync.RWMutex
	clientConfig *ClientConfig
	senders      map[SenderKey]send.Sender
}

// Configure requires that either the path or DB is sent so that it can construct the
// evergreen settings. If both are sent, the settings will be from the file
func (e *envState) Configure(ctx context.Context, confPath string, db *DBSettings) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// make sure we don't reconfigure (and leak resources)
	if e.settings != nil {
		return errors.New("cannot reconfigre a configured environment")
	}

	if db != nil && confPath == "" {
		if err := e.initDB(*db); err != nil {
			return errors.Wrap(err, "error configuring db")
		}
	}
	if err := e.initSettings(confPath); err != nil {
		return errors.WithStack(err)
	}
	if db != nil && confPath == "" {
		e.settings.Database = *db
	}

	catcher := grip.NewBasicCatcher()
	if e.session == nil {
		catcher.Add(e.initDB(e.settings.Database))
	}
	catcher.Add(e.createQueues(ctx))
	catcher.Extend(e.initQueues(ctx))
	catcher.Add(e.initClientConfig())
	catcher.Add(e.initSenders())
	if confPath != "" {
		catcher.Add(e.persistSettings())
	}

	return catcher.Resolve()
}

func (e *envState) initSettings(path string) error {
	// read configuration from either the file or DB and validate
	// if the file path is blank, the DB session must be configured already

	var err error

	if e.settings == nil {
		// this helps us test the validate method
		if path != "" {
			e.settings, err = NewSettings(path)
			if err != nil {
				return errors.Wrap(err, "problem getting settings from file")
			}
		} else {
			e.settings, err = GetConfig()
			if err != nil {
				return errors.Wrap(err, "problem getting settings from DB")
			}
		}
	}
	if e.settings == nil {
		return errors.New("unable to get settings from file and DB")
	}

	if err = e.settings.Validate(); err != nil {
		return errors.Wrap(err, "problem validating settings")
	}

	return nil
}

func (e *envState) initDB(settings DBSettings) error {
	if legacyDB.HasGlobalSessionProvider() {
		grip.Warning("database session configured; reconfiguring")
	}

	// set up the database connection configuration using the
	// legacy session factory mechanism. in the future the
	// environment can and should be the only provider of database
	// sessions.
	sf := CreateSession(settings)
	legacyDB.SetGlobalSessionProvider(sf)

	var err error

	e.session, _, err = sf.GetSession()

	return errors.Wrap(err, "problem getting database session")
}

func (e *envState) createQueues(ctx context.Context) error {
	// configure the local-only (memory-backed) queue.
	e.localQueue = queue.NewLocalLimitedSize(e.settings.Amboy.PoolSizeLocal, e.settings.Amboy.LocalStorage)

	// configure the remote mongodb-backed amboy
	// queue.
	opts := queue.DefaultMongoDBOptions()
	opts.URI = e.settings.Database.Url
	opts.DB = e.settings.Amboy.DB
	opts.Priority = true

	qmdb, err := queue.OpenNewMongoDBDriver(ctx, e.settings.Amboy.Name, opts, e.session)
	if err != nil {
		return errors.Wrap(err, "problem setting queue backend")
	}
	rq := queue.NewRemoteUnordered(e.settings.Amboy.PoolSizeRemote)
	if err = rq.SetDriver(qmdb); err != nil {
		return errors.WithStack(err)
	}
	e.remoteQueue = rq

	return nil
}

func (e *envState) initQueues(ctx context.Context) []error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(e.localQueue.Start(ctx))
	catcher.Add(e.remoteQueue.Start(ctx))

	return catcher.Errors()
}

func (e *envState) initClientConfig() (err error) {
	if e.settings == nil {
		return errors.New("no settings object, cannot build client configuration")
	}
	e.clientConfig, err = getClientConfig(e.settings.Ui.Url)
	if err == nil && len(e.clientConfig.ClientBinaries) == 0 {
		grip.Warning("No clients are available for this server")
	}
	return errors.WithStack(err)
}

func (e *envState) initSenders() (err error) {
	e.senders = map[SenderKey]send.Sender{}
	if e.settings == nil {
		return errors.New("no settings object, cannot build senders")
	}

	levelInfo := send.LevelInfo{
		Default:   level.Notice,
		Threshold: level.Notice,
	}

	if e.settings.Notify.SMTP != nil {
		smtp := e.settings.Notify.SMTP
		opts := send.SMTPOptions{
			Name:              "evergreen",
			Server:            smtp.Server,
			Port:              smtp.Port,
			UseSSL:            smtp.UseSSL,
			Username:          smtp.Username,
			Password:          smtp.Password,
			From:              smtp.From,
			PlainTextContents: false,
			NameAsSubject:     true,
		}
		if len(smtp.AdminEmail) == 0 {
			if err := opts.AddRecipient("", "test@domain.invalid"); err != nil {
				return errors.Wrap(err, "failed to setup email logger")
			}

		} else {
			for i := range smtp.AdminEmail {
				if err := opts.AddRecipient("", smtp.AdminEmail[i]); err != nil {
					return errors.Wrap(err, "failed to setup email logger")
				}
			}
		}
		sender, err := send.NewSMTPLogger(&opts, levelInfo)
		if err != nil {
			return errors.Wrap(err, "Failed to setup email logger")
		}
		e.senders[SenderEmail] = sender
	}

	githubToken, err := e.settings.GetGithubOauthToken()
	if err == nil && len(githubToken) > 0 {
		sender, err := send.NewGithubStatusLogger("evergreen", &send.GithubOptions{
			Token: githubToken,
		}, "")
		if err != nil {
			return errors.Wrap(err, "Failed to setup github status logger")
		}
		e.senders[SenderGithubStatus] = sender
	}

	if jira := &e.settings.Jira; len(jira.GetHostURL()) != 0 {
		sender, err := send.NewJiraLogger(&send.JiraOptions{
			Name:     "evergreen",
			BaseURL:  jira.GetHostURL(),
			Username: jira.Username,
			Password: jira.Password,
		}, levelInfo)
		if err != nil {
			return errors.Wrap(err, "Failed to setup jira issue logger")
		}
		e.senders[SenderJIRAIssue] = sender

		sender, err = send.NewJiraCommentLogger("", &send.JiraOptions{
			Name:     "evergreen",
			BaseURL:  jira.GetHostURL(),
			Username: jira.Username,
			Password: jira.Password,
		}, levelInfo)
		if err != nil {
			return errors.Wrap(err, "Failed to setup jira comment logger")
		}
		e.senders[SenderJIRAComment] = sender
	}

	if slack := &e.settings.Slack; len(slack.Token) != 0 {
		sender, err := send.NewSlackLogger(&send.SlackOptions{
			Channel: "#",
			Name:    "evergreen",
		}, slack.Token, levelInfo)
		if err != nil {
			return errors.Wrap(err, "Failed to setup slack logger")
		}
		e.senders[SenderSlack] = sender
	}

	sender, err := util.NewEvergreenWebhookLogger()
	if err != nil {
		return errors.Wrap(err, "Failed to setup evergreen webhook logger")
	}
	e.senders[SenderEvergreenWebhook] = sender

	for _, s := range e.senders {
		s.SetLevel(levelInfo)
	}

	return nil
}

type bbProject struct {
	TicketCreateProject  string   `mapstructure:"ticket_create_project" bson:"ticket_create_project"`
	TicketSearchProjects []string `mapstructure:"ticket_search_projects" bson:"ticket_search_projects"`
}

func (e *envState) persistSettings() error {
	if e.settings == nil {
		return errors.New("no settings object, cannot persist to DB")
	}

	// this is a hacky workaround to any plugins that have fields that are maps, since
	// deserializing these fields from yaml does not maintain the typing information
	var copy Settings
	registeredTypes := []interface{}{
		map[interface{}]interface{}{},
		map[string]interface{}{},
		[]interface{}{},
		[]util.KeyValuePair{},
	}
	err := deepCopy(*e.settings, &copy, registeredTypes)
	if err != nil {
		return errors.Wrap(err, "problem copying settings")
	}

	gob.Register(map[interface{}]interface{}{})
	for pluginName, plugin := range copy.Plugins {
		if pluginName == "buildbaron" {
			for fieldName, field := range plugin {
				if fieldName == "projects" {
					var projects map[string]bbProject
					err := mapstructure.Decode(field, &projects)
					if err != nil {
						return errors.Wrap(err, "problem decoding buildbaron projects")
					}
					plugin[fieldName] = projects
				}
			}
		}
		if pluginName == "dashboard" {
			for fieldName, field := range plugin {
				if fieldName == "branches" {
					var branches map[string][]string
					err := mapstructure.Decode(field, &branches)
					if err != nil {
						return errors.Wrap(err, "problem decoding dashboard branches")
					}
					plugin[fieldName] = branches
				}
			}
		}
	}

	return errors.WithStack(UpdateConfig(&copy))
}

func (e *envState) Settings() *Settings {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.settings
}

func (e *envState) LocalQueue() amboy.Queue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.localQueue
}

func (e *envState) RemoteQueue() amboy.Queue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.remoteQueue
}

func (e *envState) Session() db.Session {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return db.WrapSession(e.session.Copy())
}

func (e *envState) ClientConfig() *ClientConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.clientConfig == nil {
		return nil
	}

	config := *e.clientConfig
	return &config
}

func (e *envState) GetSender(key SenderKey) (send.Sender, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	sender, ok := e.senders[key]
	if !ok {
		return nil, errors.Errorf("unknown sender key %v", key)
	}

	return sender, nil
}

// getClientConfig should be called once at startup and looks at the
// current environment and loads all currently available client
// binaries for use by the API server in presenting the settings page.
//
// If there are no built clients, this returns an empty config
// version, but does *not* error.
func getClientConfig(baseURL string) (*ClientConfig, error) {
	c := &ClientConfig{}
	c.LatestRevision = ClientVersion

	root := filepath.Join(FindEvergreenHome(), ClientDirectory)

	if _, err := os.Stat(root); os.IsNotExist(err) {
		grip.Warningf("client directory '%s' does not exist, creating empty "+
			"directory and continuing with caution", root)
		grip.Error(os.MkdirAll(root, 0755))
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || info.Name() == "version" {
			return nil
		}

		parts := strings.Split(path, string(filepath.Separator))
		buildInfo := strings.Split(parts[len(parts)-2], "_")

		c.ClientBinaries = append(c.ClientBinaries, ClientBinary{
			URL: fmt.Sprintf("%s/%s/%s", baseURL, ClientDirectory,
				strings.Join(parts[len(parts)-2:], "/")),
			OS:   buildInfo[0],
			Arch: buildInfo[1],
		})

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "problem finding client binaries")
	}

	return c, nil
}

// copy of deepCopy in util package
func deepCopy(src, copy interface{}, registeredTypes []interface{}) error {
	for _, t := range registeredTypes {
		gob.Register(t)
	}
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	dec := gob.NewDecoder(&buff)
	err := enc.Encode(src)
	if err != nil {
		return errors.Wrap(err, "error encoding source")
	}
	return errors.Wrap(dec.Decode(copy), "error decoding copy")
}
