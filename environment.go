package evergreen

import (
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/logger"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	globalEnv     Environment
	globalEnvLock *sync.RWMutex
)

const (
	// duration of wait time during queue chut down.
	queueShutdownWaitInterval = 10 * time.Millisecond
	queueShutdownWaitTimeout  = 10 * time.Second
)

func init() { globalEnvLock = &sync.RWMutex{} }

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
func GetEnvironment() Environment {
	return globalEnv
}

func SetEnvironment(env Environment) {
	globalEnvLock.Lock()
	defer globalEnvLock.Unlock()

	globalEnv = env
}

// Environment provides application-level services (e.g. databases,
// configuration, queues.
type Environment interface {
	// Returns the settings object. The settings object is not
	// necessarily safe for concurrent access.
	Settings() *Settings
	Context() (context.Context, context.CancelFunc)

	Session() db.Session
	Client() *mongo.Client
	DB() *mongo.Database

	// The Environment provides access to two queues, a
	// local-process level queue that is not persisted between
	// runs and a remote shared queue that all processes can use
	// to distribute work amongst the application tier
	//
	// It also exposes a queue group, which permits dynamically
	// generating queues at runtime.
	//
	// The LocalQueue is not durable, and results aren't available
	// between process restarts. The RemoteQueue is not
	// (generally) started by default.
	LocalQueue() amboy.Queue
	RemoteQueue() amboy.Queue
	RemoteQueueGroup() amboy.QueueGroup

	// Jasper is a process manager for running external
	// commands. Every process has a manager service.
	JasperManager() jasper.Manager

	// ClientConfig provides access to a list of the latest evergreen
	// clients, that this server can serve to users
	ClientConfig() *ClientConfig

	// SaveConfig persists the configuration settings.
	SaveConfig() error

	// GetSender provides a grip Sender configured with the environment's
	// settings. These Grip senders must be used with Composers that specify
	// all message details.
	GetSender(SenderKey) (send.Sender, error)
	SetSender(SenderKey, send.Sender) error

	// RegisterCloser adds a function object to an internal
	// tracker to be called by the Close method before process
	// termination. The ID is used in reporting, but must be
	// unique or a new closer could overwrite an existing closer
	// in some implementations.
	RegisterCloser(string, func(context.Context) error)
	// Close calls all registered closers in the environment.
	Close(context.Context) error
}

// NewEnvironment constructs an Environment instance, establishing a
// new connection to the database, and creating a new set of worker
// queues.
//
// When NewEnvironment returns without an error, you should assume
// that the queues have been started, there was no issue
// establishing a connection to the database, and that the
// local and remote queues have started.
//
// NewEnvironment requires that either the path or DB is sent so that
// if both are specified, the settings are read from the file.
func NewEnvironment(ctx context.Context, confPath string, db *DBSettings) (Environment, error) {
	ctx, cancel := context.WithCancel(ctx)
	e := &envState{
		ctx:     ctx,
		senders: map[SenderKey]send.Sender{},
		closers: map[string]func(context.Context) error{
			"root-context": func(_ context.Context) error {
				cancel()
				return nil
			},
		},
	}

	if db != nil && confPath == "" {
		if err := e.initDB(ctx, *db); err != nil {
			return nil, errors.Wrap(err, "error configuring db")
		}
		e.dbName = db.DB
	}

	if err := e.initSettings(confPath); err != nil {
		return nil, errors.WithStack(err)
	}

	if db != nil && confPath == "" {
		e.settings.Database = *db
	}

	e.dbName = e.settings.Database.DB

	catcher := grip.NewBasicCatcher()
	if e.client == nil {
		catcher.Add(e.initDB(ctx, e.settings.Database))
	}

	catcher.Add(e.initJasper())
	catcher.Add(e.initSenders(ctx))
	catcher.Add(e.createLocalQueue(ctx))
	catcher.Add(e.createApplicationQueue(ctx))
	catcher.Add(e.createNotificationQueue(ctx))
	catcher.Add(e.createGenerateTasksQueue(ctx))
	catcher.Extend(e.initQueues(ctx))

	if catcher.HasErrors() {
		return nil, errors.WithStack(catcher.Resolve())

	}
	return e, nil
}

type ClientBinary struct {
	Arch string `yaml:"arch" json:"arch"`
	OS   string `yaml:"os" json:"os"`
	URL  string `yaml:"url" json:"url"`
}

type ClientConfig struct {
	ClientBinaries []ClientBinary `yaml:"client_binaries" json:"ClientBinaries"`
	LatestRevision string         `yaml:"latest_revision" json:"LatestRevision"`
}

type envState struct {
	remoteQueue        amboy.Queue
	localQueue         amboy.Queue
	remoteQueueGroup   amboy.QueueGroup
	notificationsQueue amboy.Queue
	ctx                context.Context
	jasperManager      jasper.Manager
	settings           *Settings
	dbName             string
	client             *mongo.Client
	mu                 sync.RWMutex
	clientConfig       *ClientConfig
	closers            map[string]func(context.Context) error
	senders            map[SenderKey]send.Sender
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
			e.settings, err = BootstrapConfig(e)
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

func (e *envState) initDB(ctx context.Context, settings DBSettings) error {
	var err error
	opts := options.Client().ApplyURI(settings.Url).SetWriteConcern(settings.WriteConcernSettings.Resolve()).SetConnectTimeout(5 * time.Second)
	e.client, err = mongo.NewClient(opts)
	if err != nil {
		return errors.Wrap(err, "problem constructing database")
	}

	if err = e.client.Connect(ctx); err != nil {
		return errors.Wrap(err, "problem connecting to the database")
	}

	return nil
}

func (e *envState) Context() (context.Context, context.CancelFunc) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return context.WithCancel(e.ctx)
}

func (e *envState) Client() *mongo.Client {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.client
}

func (e *envState) DB() *mongo.Database {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.client.Database(e.dbName)
}

func (e *envState) createLocalQueue(ctx context.Context) error {
	// configure the local-only (memory-backed) queue.
	e.localQueue = queue.NewLocalLimitedSize(e.settings.Amboy.PoolSizeLocal, e.settings.Amboy.LocalStorage)
	if err := e.localQueue.SetRunner(pool.NewAbortablePool(e.settings.Amboy.PoolSizeLocal, e.localQueue)); err != nil {
		return errors.Wrap(err, "problem configuring worker pool for local queue")
	}

	e.closers["background-local-queue"] = func(ctx context.Context) error {
		e.localQueue.Runner().Close()
		return nil
	}

	return nil
}

func (e *envState) createApplicationQueue(ctx context.Context) error {
	// configure the remote mongodb-backed amboy
	// queue.
	opts := queue.DefaultMongoDBOptions()
	opts.URI = e.settings.Database.Url
	opts.DB = e.settings.Amboy.DB
	opts.Priority = true

	qmdb, err := queue.OpenNewMongoDriver(ctx, e.settings.Amboy.Name, opts, e.client)
	if err != nil {
		return errors.Wrap(err, "problem setting main queue backend")
	}
	rq := queue.NewRemoteUnordered(e.settings.Amboy.PoolSizeRemote)
	if err = rq.SetDriver(qmdb); err != nil {
		return errors.WithStack(err)
	}
	if err = rq.SetRunner(pool.NewAbortablePool(e.settings.Amboy.PoolSizeRemote, rq)); err != nil {
		return errors.Wrap(err, "problem configuring worker pool for main remote queue")
	}
	e.remoteQueue = rq
	e.closers["application-queue"] = func(ctx context.Context) error {
		e.remoteQueue.Runner().Close()
		return nil
	}

	return nil
}

func (e *envState) createGenerateTasksQueue(ctx context.Context) error {
	opts := queue.DefaultMongoDBOptions()
	opts.URI = e.settings.Database.Url
	opts.DB = e.settings.Amboy.DB
	opts.Priority = true

	remoteQueuGroupOpts := queue.RemoteQueueGroupOptions{
		Client: e.client,
		Constructor: func(_ context.Context) (queue.Remote, error) {
			q := queue.NewRemoteUnordered(1)
			if err := q.SetRunner(pool.NewAbortablePool(1, q)); err != nil {
				return nil, errors.WithStack(err)
			}
			return q, nil
		},
		MongoOptions: queue.DefaultMongoDBOptions(),
		Prefix:       "gen",
		TTL:          7 * 24 * time.Hour,
	}
	remoteQueueGroup, err := queue.NewRemoteQueueGroup(ctx, remoteQueuGroupOpts)
	if err != nil {
		return errors.Wrap(err, "problem constructing remote queue group")
	}
	e.remoteQueueGroup = remoteQueueGroup

	e.closers["generate-tasks"] = func(ctx context.Context) error {
		closerCtx, _ := context.WithTimeout(context.Background(), 30*time.Second) // nolint
		e.remoteQueueGroup.Close(closerCtx)
		return nil
	}

	return nil
}

func (e *envState) createNotificationQueue(ctx context.Context) error {
	// Notifications queue w/ moving weight avg pool
	e.notificationsQueue = queue.NewLocalLimitedSize(len(e.senders), e.settings.Amboy.LocalStorage)

	runner, err := pool.NewMovingAverageRateLimitedWorkers(e.settings.Amboy.PoolSizeLocal,
		e.settings.Notify.BufferTargetPerInterval,
		time.Duration(e.settings.Notify.BufferIntervalSeconds)*time.Second,
		e.notificationsQueue)
	if err != nil {
		return errors.Wrap(err, "Failed to make notifications queue runner")
	}
	if err = e.notificationsQueue.SetRunner(runner); err != nil {
		return errors.Wrap(err, "failed to set notifications queue runner")
	}
	rootSenders := []send.Sender{}
	for _, s := range e.senders {
		rootSenders = append(rootSenders, s)
	}

	e.closers["notification-queue"] = func(ctx context.Context) error {
		var cancel context.CancelFunc
		catcher := grip.NewBasicCatcher()
		ctx, cancel = context.WithTimeout(ctx, queueShutdownWaitTimeout)
		defer cancel()
		if !amboy.WaitCtxInterval(ctx, e.notificationsQueue, queueShutdownWaitInterval) {
			grip.Critical(message.Fields{
				"message": "pending jobs failed to finish",
				"queue":   "notifications",
				"status":  e.notificationsQueue.Stats(),
			})
			catcher.Add(errors.New("failed to stop with running jobs"))
		}

		e.notificationsQueue.Runner().Close()

		grip.Debug(message.Fields{
			"message":     "closed notification queue",
			"num_senders": len(rootSenders),
			"errors":      catcher.HasErrors(),
		})

		for _, s := range rootSenders {
			catcher.Add(s.Close())
		}
		grip.Debug(message.Fields{
			"message":     "closed all root senders",
			"num_senders": len(rootSenders),
			"errors":      catcher.HasErrors(),
		})

		return catcher.Resolve()
	}

	for k := range e.senders {
		e.senders[k] = logger.MakeQueueSender(e.notificationsQueue, e.senders[k])
	}

	return nil
}

func (e *envState) initQueues(ctx context.Context) []error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(e.localQueue == nil, "local queue is not defined")
	catcher.NewWhen(e.notificationsQueue == nil, "notification queue is not defined")
	catcher.NewWhen(e.notificationsQueue == nil, "notification queue is not defined")

	if e.localQueue != nil {
		catcher.Add(e.localQueue.Start(ctx))
	}

	if e.notificationsQueue != nil {
		catcher.Add(e.notificationsQueue.Start(ctx))
	}

	return catcher.Errors()
}

func (e *envState) initClientConfig() {
	if e.settings == nil {
		grip.Critical("no settings object, cannot build client configuration")
		return
	}
	var err error

	e.clientConfig, err = getClientConfig(e.settings.Ui.Url)

	if err != nil {
		grip.Critical(message.WrapError(err, message.Fields{
			"message": "problem finding local clients",
			"cause":   "infrastructure configuration issue",
			"impact":  "agent deploys",
		}))
	} else if len(e.clientConfig.ClientBinaries) == 0 {
		grip.Critical("No clients are available for this server")
	}
}

func (e *envState) initSenders(ctx context.Context) error {
	if e.settings == nil {
		return errors.New("no settings object, cannot build senders")
	}

	levelInfo := send.LevelInfo{
		Default:   level.Notice,
		Threshold: level.Notice,
	}

	if e.settings.Notify.SMTP.From != "" {
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

	var sender send.Sender
	githubToken, err := e.settings.GetGithubOauthToken()
	if err == nil && len(githubToken) > 0 {
		// Github Status
		sender, err = send.NewGithubStatusLogger("evergreen", &send.GithubOptions{
			Token: githubToken,
		}, "")
		if err != nil {
			return errors.Wrap(err, "Failed to setup github status logger")
		}
		e.senders[SenderGithubStatus] = sender
	}

	if jira := &e.settings.Jira; len(jira.GetHostURL()) != 0 {
		sender, err = send.NewJiraLogger(&send.JiraOptions{
			Name:         "evergreen",
			BaseURL:      jira.GetHostURL(),
			Username:     jira.Username,
			Password:     jira.Password,
			UseBasicAuth: true,
		}, levelInfo)
		if err != nil {
			return errors.Wrap(err, "Failed to setup jira issue logger")
		}
		e.senders[SenderJIRAIssue] = sender

		sender, err = send.NewJiraCommentLogger("", &send.JiraOptions{
			Name:         "evergreen",
			BaseURL:      jira.GetHostURL(),
			Username:     jira.Username,
			Password:     jira.Password,
			UseBasicAuth: true,
		}, levelInfo)
		if err != nil {
			return errors.Wrap(err, "Failed to setup jira comment logger")
		}
		e.senders[SenderJIRAComment] = sender
	}

	if slack := &e.settings.Slack; len(slack.Token) != 0 {
		// this sender is initialised with an invalid channel. Any
		// messages sent with it that do not use message.SlackMessage
		// will not be received
		sender, err = send.NewSlackLogger(&send.SlackOptions{
			Channel:  "#",
			Name:     "evergreen",
			Username: "Evergreen",
			IconURL:  fmt.Sprintf("%s/static/img/evergreen_green_150x150.png", e.settings.Ui.Url),
		}, slack.Token, levelInfo)
		if err != nil {
			return errors.Wrap(err, "Failed to setup slack logger")
		}
		e.senders[SenderSlack] = sender
	}

	sender, err = util.NewEvergreenWebhookLogger()
	if err != nil {
		return errors.Wrap(err, "Failed to setup evergreen webhook logger")
	}
	e.senders[SenderEvergreenWebhook] = sender

	catcher := grip.NewBasicCatcher()
	for name, s := range e.senders {
		catcher.Add(s.SetLevel(levelInfo))
		catcher.Add(s.SetErrorHandler(util.MakeNotificationErrorHandler(name.String())))
	}

	return catcher.Resolve()
}

func (e *envState) initJasper() error {
	jpm, err := jasper.NewLocalManager(true)
	if err != nil {
		return errors.WithStack(err)
	}

	e.jasperManager = jpm

	e.closers["jasper-manaer"] = func(ctx context.Context) error {
		return errors.WithStack(jpm.Close(ctx))
	}

	return nil
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

func (e *envState) RemoteQueueGroup() amboy.QueueGroup {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.remoteQueueGroup
}

func (e *envState) Session() db.Session {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return db.WrapClient(e.ctx, e.client).Clone()
}

func (e *envState) ClientConfig() *ClientConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.clientConfig == nil {
		e.initClientConfig()
		if e.clientConfig == nil {
			return nil
		}
	}

	config := *e.clientConfig
	return &config
}

type BuildBaronProject struct {
	TicketCreateProject  string   `mapstructure:"ticket_create_project" bson:"ticket_create_project"`
	TicketSearchProjects []string `mapstructure:"ticket_search_projects" bson:"ticket_search_projects"`

	// The BF Suggestion server as a source of suggestions is only enabled for projects where BFSuggestionServer isn't the empty string.
	BFSuggestionServer      string `mapstructure:"bf_suggestion_server" bson:"bf_suggestion_server"`
	BFSuggestionUsername    string `mapstructure:"bf_suggestion_username" bson:"bf_suggestion_username"`
	BFSuggestionPassword    string `mapstructure:"bf_suggestion_password" bson:"bf_suggestion_password"`
	BFSuggestionTimeoutSecs int    `mapstructure:"bf_suggestion_timeout_secs" bson:"bf_suggestion_timeout_secs"`
	BFSuggestionFeaturesURL string `mapstructure:"bf_suggestion_features_url" bson:"bf_suggestion_features_url"`
}

func (e *envState) SaveConfig() error {
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
	err := util.DeepCopy(*e.settings, &copy, registeredTypes)
	if err != nil {
		return errors.Wrap(err, "problem copying settings")
	}

	gob.Register(map[interface{}]interface{}{})
	for pluginName, plugin := range copy.Plugins {
		if pluginName == "buildbaron" {
			for fieldName, field := range plugin {
				if fieldName == "projects" {
					var projects map[string]BuildBaronProject
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

func (e *envState) GetSender(key SenderKey) (send.Sender, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	sender, ok := e.senders[key]
	if !ok {
		return nil, errors.Errorf("unknown sender key %v", key)
	}

	return sender, nil
}

func (e *envState) SetSender(key SenderKey, impl send.Sender) error {
	if impl == nil {
		return errors.New("cannot add a nil sender")
	}

	if err := key.Validate(); err != nil {
		return errors.WithStack(err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.senders[key] = impl

	return nil
}

func (e *envState) RegisterCloser(name string, closer func(context.Context) error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.closers[name]; ok {
		grip.Critical(message.Fields{
			"closer":  name,
			"message": "duplicate closer registered",
			"cause":   "programmer error",
		})
	}
	e.closers[name] = closer
}

func (e *envState) Close(ctx context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// TODO we could, in the future call all closers in but that
	// would require more complex waiting and timeout logic

	deadline, _ := ctx.Deadline()
	catcher := grip.NewBasicCatcher()
	wg := &sync.WaitGroup{}
	for n, closer := range e.closers {
		if closer == nil {
			continue
		}

		wg.Add(1)
		go func(name string, close func(context.Context) error) {
			defer wg.Done()
			grip.Info(message.Fields{
				"message":      "calling closer",
				"closer":       name,
				"timeout_secs": time.Until(deadline),
				"deadline":     deadline,
			})
			catcher.Add(close(ctx))
		}(n, closer)
	}

	wg.Wait()
	return catcher.Resolve()
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

		if info.IsDir() || !strings.Contains(info.Name(), "evergreen") {
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

func (e *envState) JasperManager() jasper.Manager {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.jasperManager
}
