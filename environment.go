package evergreen

import (
	"context"
	"encoding/gob"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/logger"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/apm"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

var (
	globalEnv     Environment
	globalEnvLock *sync.RWMutex

	// don't ever access this directly except from testutil
	PermissionSystemDisabled = false
)

const (
	// duration of wait time during queue chut down.
	queueShutdownWaitInterval = 10 * time.Millisecond
	queueShutdownWaitTimeout  = 10 * time.Second
	githubTokenTimeout        = 1 * time.Hour

	RoleCollection  = "roles"
	ScopeCollection = "scopes"
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
	// Settings returns the cached version of the admin settings as a settings object.
	// The settings object is not necessarily safe for concurrent access.
	// Use GetConfig() to access the settings object from the DB.
	Settings() *Settings
	Context() (context.Context, context.CancelFunc)

	Session() db.Session
	Client() *mongo.Client
	DB() *mongo.Database

	// The Environment provides access to several amboy queues for
	// processing background work in the context of the Evergreen
	// application.
	//
	// The LocalQueue provides process-local execution, to support
	// reporting and cleanup operations local to a single instance
	// of the evergreen application.  These queues are not
	// durable, and job data are not available between application
	// restarts.
	//
	// The RemoteQueue provides a single queue with many
	// workers, distributed across all application servers. Each
	// application dedicates a moderate pool of workers, and work
	// enters this queue from periodic operations
	// (e.g. "cron-like") as well as work that is submitted as a
	// result of user requests. The service queue is
	// mixed-workload.
	//
	// The RemoteQueueGroup provides logically distinct
	// application queues in situations where we need to isolate
	// workloads between queues. The queues are backed remotely, which
	// means that their work persists between restarts.
	LocalQueue() amboy.Queue
	RemoteQueue() amboy.Queue
	RemoteQueueGroup() amboy.QueueGroup

	// Jasper is a process manager for running external
	// commands. Every process has a manager service.
	JasperManager() jasper.Manager
	CertificateDepot() certdepot.Depot

	// ClientConfig provides access to a list of the latest evergreen
	// clients, that this server can serve to users
	ClientConfig() *ClientConfig

	// SaveConfig persists the configuration settings.
	SaveConfig(context.Context) error

	// GetSender provides a grip Sender configured with the environment's
	// settings. These Grip senders must be used with Composers that specify
	// all message details.
	GetSender(SenderKey) (send.Sender, error)
	SetSender(SenderKey, send.Sender) error

	// GetGitHubSender provides a grip Sender configured with the given
	// owner and repo information.
	GetGitHubSender(string, string) (send.Sender, error)

	// RegisterCloser adds a function object to an internal
	// tracker to be called by the Close method before process
	// termination. The ID is used in reporting, but must be
	// unique or a new closer could overwrite an existing closer
	// in some implementations.
	RegisterCloser(string, bool, func(context.Context) error)
	// Close calls all registered closers in the environment.
	Close(context.Context) error

	// RoleManager returns an interface that can be used to interact with roles and permissions
	RoleManager() gimlet.RoleManager

	// UserManager returns the global user manager for authentication.
	UserManager() gimlet.UserManager
	SetUserManager(gimlet.UserManager)
	// UserManagerInfo returns the information about the user manager.
	UserManagerInfo() UserManagerInfo
	SetUserManagerInfo(UserManagerInfo)
	// ShutdownSequenceStarted is true iff the shutdown sequence has been started
	ShutdownSequenceStarted() bool
	SetShutdown()
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
		ctx:                     ctx,
		senders:                 map[SenderKey]send.Sender{},
		shutdownSequenceStarted: false,
	}
	defer func() {
		e.RegisterCloser("root-context", false, func(_ context.Context) error {
			cancel()
			return nil
		})
	}()

	if db != nil && confPath == "" {
		if err := e.initDB(ctx, *db); err != nil {
			return nil, errors.Wrap(err, "initializing DB")
		}
		e.dbName = db.DB
		// Persist the environment early so the db will be available for initSettings.
		SetEnvironment(e)
	}

	if err := e.initSettings(ctx, confPath); err != nil {
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
	catcher.Add(e.initDepot(ctx))
	catcher.Add(e.initSenders(ctx))
	catcher.Add(e.createLocalQueue(ctx))
	catcher.Add(e.createRemoteQueues(ctx))
	catcher.Add(e.createNotificationQueue(ctx))
	catcher.Add(e.setupRoleManager())
	catcher.Add(e.initTracer(ctx))
	catcher.Extend(e.initQueues(ctx))

	if catcher.HasErrors() {
		return nil, errors.WithStack(catcher.Resolve())

	}
	return e, nil
}

type envState struct {
	remoteQueue             amboy.Queue
	localQueue              amboy.Queue
	remoteQueueGroup        amboy.QueueGroup
	notificationsQueue      amboy.Queue
	ctx                     context.Context
	jasperManager           jasper.Manager
	depot                   certdepot.Depot
	settings                *Settings
	dbName                  string
	client                  *mongo.Client
	mu                      sync.RWMutex
	clientConfig            *ClientConfig
	closers                 []closerOp
	senders                 map[SenderKey]send.Sender
	githubSenders           map[string]cachedGitHubSender
	roleManager             gimlet.RoleManager
	userManager             gimlet.UserManager
	userManagerInfo         UserManagerInfo
	shutdownSequenceStarted bool
}

// UserManagerInfo lists properties of the UserManager regarding its support for
// certain features.
// TODO: this should probably be removed by refactoring the optional methods in
// the gimlet.UserManager.
type UserManagerInfo struct {
	CanClearTokens bool
	CanReauthorize bool
}

type closerOp struct {
	name       string
	background bool
	closerFn   func(context.Context) error
}

// cachedGitHubSender stores a GitHub sender and the time it was created
// because GitHub app tokens, and by extension, the senders, will expire
// one hour after creation.
type cachedGitHubSender struct {
	sender send.Sender
	time   time.Time
}

func (e *envState) initSettings(ctx context.Context, path string) error {
	// read configuration from either the file or DB and validate
	// if the file path is blank, the DB session must be configured already

	var err error

	if e.settings == nil {
		// this helps us test the validate method
		if path != "" {
			e.settings, err = NewSettings(path)
			if err != nil {
				return errors.Wrap(err, "getting config settings from file")
			}
		} else {
			e.settings, err = GetConfig(ctx)
			if err != nil {
				return errors.Wrap(err, "getting config settings from DB")
			}
		}
	}
	if e.settings == nil {
		return errors.New("unable to get settings from file or DB")
	}

	if err = e.settings.Validate(); err != nil {
		return errors.Wrap(err, "validating settings")
	}

	return nil
}

// getCollectionName extracts the collection name from a command.
// Returns an error if the command is deformed or is not a CRUD command.
func getCollectionName(command bson.Raw) (string, error) {
	element, err := command.IndexErr(0)
	if err != nil {
		return "", errors.Wrap(err, "command has no first element")
	}
	v, err := element.ValueErr()
	if err != nil {
		return "", errors.Wrap(err, "getting element value")
	}
	if v.Type != bsontype.String {
		return "", errors.Errorf("element value was of unexpected type '%s'", v.Type)
	}
	return v.StringValue(), nil
}

// redactSensitiveCollections satisfies the apm.CommandTransformer interface.
// Returns an empty string when the command is a CRUD command on a sensitive collection
// or if we can't determine the collection the command is on.
func redactSensitiveCollections(command bson.Raw) string {
	collectionName, err := getCollectionName(command)
	if err != nil || utility.StringSliceContains(sensitiveCollections, collectionName) {
		return ""
	}

	b, _ := bson.MarshalExtJSON(command, false, false)
	return string(b)
}

func (e *envState) initDB(ctx context.Context, settings DBSettings) error {
	opts := options.Client().ApplyURI(settings.Url).SetWriteConcern(settings.WriteConcernSettings.Resolve()).
		SetReadConcern(settings.ReadConcernSettings.Resolve()).
		SetConnectTimeout(5 * time.Second).
		SetMonitor(apm.NewMonitor(apm.WithCommandAttributeDisabled(false), apm.WithCommandAttributeTransformer(redactSensitiveCollections)))

	if settings.HasAuth() {
		ymlUser, ymlPwd, err := settings.GetAuth()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem getting auth settings from YAML file, attempting to connect without auth",
			}))
		}
		if err == nil && ymlUser != "" {
			credential := options.Credential{
				Username: ymlUser,
				Password: ymlPwd,
			}

			opts.SetAuth(credential)
		}
	}

	var err error
	e.client, err = mongo.Connect(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "connecting to the Evergreen DB")
	}

	return nil
}

func (e *envState) createRemoteQueues(ctx context.Context) error {
	url := e.settings.Amboy.DBConnection.URL
	if url == "" {
		url = DefaultAmboyDatabaseURL
	}

	opts := options.Client().
		ApplyURI(url).
		SetConnectTimeout(5 * time.Second).
		SetReadPreference(readpref.Primary()).
		SetReadConcern(e.settings.Database.ReadConcernSettings.Resolve()).
		SetWriteConcern(e.settings.Database.WriteConcernSettings.Resolve()).
		SetMonitor(apm.NewMonitor(apm.WithCommandAttributeDisabled(false)))

	if e.settings.Amboy.DBConnection.Username != "" && e.settings.Amboy.DBConnection.Password != "" {
		opts.SetAuth(options.Credential{
			Username: e.settings.Amboy.DBConnection.Username,
			Password: e.settings.Amboy.DBConnection.Password,
		})
	}

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "connecting to the Amboy database")
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(e.createApplicationQueue(ctx, client))
	catcher.Add(e.createRemoteQueueGroup(ctx, client))
	return catcher.Resolve()
}

func (e *envState) Context() (context.Context, context.CancelFunc) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return context.WithCancel(e.ctx)
}

func (e *envState) SetShutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shutdownSequenceStarted = true
}

func (e *envState) ShutdownSequenceStarted() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.shutdownSequenceStarted
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
		return errors.Wrap(err, "setting local queue worker pool")
	}

	e.RegisterCloser("background-local-queue", true, func(ctx context.Context) error {
		e.localQueue.Close(ctx)
		return nil
	})

	return nil
}

func (e *envState) createApplicationQueue(ctx context.Context, client *mongo.Client) error {
	// configure the remote mongodb-backed amboy
	// queue.
	opts := queue.DefaultMongoDBOptions()
	opts.Client = client
	opts.DB = e.settings.Amboy.DBConnection.Database
	opts.Collection = e.settings.Amboy.Name
	opts.Priority = e.settings.Amboy.RequireRemotePriority
	opts.SkipQueueIndexBuilds = true
	opts.SkipReportingIndexBuilds = true
	opts.PreferredIndexes = e.getPreferredRemoteQueueIndexes()
	opts.UseGroups = false
	opts.LockTimeout = time.Duration(e.settings.Amboy.LockTimeoutMinutes) * time.Minute
	opts.SampleSize = e.settings.Amboy.SampleSize

	retryOpts := e.settings.Amboy.Retry.RetryableQueueOptions()
	queueOpts := queue.MongoDBQueueOptions{
		NumWorkers: utility.ToIntPtr(e.settings.Amboy.PoolSizeRemote),
		DB:         &opts,
		Retryable:  &retryOpts,
	}

	rq, err := queue.NewMongoDBQueue(ctx, queueOpts)
	if err != nil {
		return errors.Wrap(err, "creating main remote queue")
	}

	if err = rq.SetRunner(pool.NewAbortablePool(e.settings.Amboy.PoolSizeRemote, rq)); err != nil {
		return errors.Wrap(err, "setting main remote queue worker pool")
	}
	e.remoteQueue = rq
	e.RegisterCloser("application-queue", false, func(ctx context.Context) error {
		e.remoteQueue.Close(ctx)
		return nil
	})

	return nil
}

func (e *envState) createRemoteQueueGroup(ctx context.Context, client *mongo.Client) error {
	opts := e.getRemoteQueueGroupDBOptions(client)

	retryOpts := e.settings.Amboy.Retry.RetryableQueueOptions()
	queueOpts := queue.MongoDBQueueOptions{
		NumWorkers: utility.ToIntPtr(e.settings.Amboy.GroupDefaultWorkers),
		DB:         &opts,
		Retryable:  &retryOpts,
	}

	perQueue, regexpQueue, err := e.getNamedRemoteQueueOptions(client)
	if err != nil {
		return errors.Wrap(err, "getting named remote queue options")
	}

	remoteQueueGroupOpts := queue.MongoDBQueueGroupOptions{
		DefaultQueue:              queueOpts,
		PerQueue:                  perQueue,
		RegexpQueue:               regexpQueue,
		BackgroundCreateFrequency: time.Duration(e.settings.Amboy.GroupBackgroundCreateFrequencyMinutes) * time.Minute,
		PruneFrequency:            time.Duration(e.settings.Amboy.GroupPruneFrequencyMinutes) * time.Minute,
		TTL:                       time.Duration(e.settings.Amboy.GroupTTLMinutes) * time.Minute,
	}

	remoteQueueGroup, err := queue.NewMongoDBSingleQueueGroup(ctx, remoteQueueGroupOpts)
	if err != nil {
		return errors.Wrap(err, "creating remote queue group")
	}
	e.remoteQueueGroup = remoteQueueGroup

	e.RegisterCloser("remote-queue-group", false, func(ctx context.Context) error {
		return errors.Wrap(e.remoteQueueGroup.Close(ctx), "waiting for remote queue group to close")
	})

	return nil
}

func (e *envState) getRemoteQueueGroupDBOptions(client *mongo.Client) queue.MongoDBOptions {
	opts := queue.DefaultMongoDBOptions()
	opts.Client = client
	opts.DB = e.settings.Amboy.DBConnection.Database
	opts.Collection = e.settings.Amboy.Name
	opts.Priority = e.settings.Amboy.RequireRemotePriority
	opts.SkipQueueIndexBuilds = true
	opts.SkipReportingIndexBuilds = true
	opts.PreferredIndexes = e.getPreferredRemoteQueueIndexes()
	opts.UseGroups = true
	opts.GroupName = e.settings.Amboy.Name
	opts.LockTimeout = time.Duration(e.settings.Amboy.LockTimeoutMinutes) * time.Minute
	return opts
}

func (e *envState) getPreferredRemoteQueueIndexes() queue.PreferredIndexOptions {
	if e.settings.Amboy.SkipPreferredIndexes {
		return queue.PreferredIndexOptions{}
	}
	return queue.PreferredIndexOptions{
		NextJob: bson.D{
			bson.E{
				Key:   "status.completed",
				Value: 1,
			},
			bson.E{
				Key:   "status.in_prog",
				Value: 1,
			},
			bson.E{
				Key:   "status.mod_ts",
				Value: 1,
			},
		},
	}
}

func (e *envState) getNamedRemoteQueueOptions(client *mongo.Client) (map[string]queue.MongoDBQueueOptions, []queue.RegexpMongoDBQueueOptions, error) {
	perQueueOpts := map[string]queue.MongoDBQueueOptions{}
	var regexpQueueOpts []queue.RegexpMongoDBQueueOptions
	for _, namedQueue := range e.settings.Amboy.NamedQueues {
		if namedQueue.Name == "" && namedQueue.Regexp == "" {
			continue
		}

		dbOpts := e.getRemoteQueueGroupDBOptions(client)
		if namedQueue.SampleSize != 0 {
			dbOpts.SampleSize = namedQueue.SampleSize
		}
		if namedQueue.LockTimeoutSeconds != 0 {
			dbOpts.LockTimeout = time.Duration(namedQueue.LockTimeoutSeconds) * time.Second
		}
		var numWorkers int
		if namedQueue.NumWorkers != 0 {
			numWorkers = namedQueue.NumWorkers
		} else {
			numWorkers = e.settings.Amboy.GroupDefaultWorkers
		}
		queueOpts := queue.MongoDBQueueOptions{
			NumWorkers: utility.ToIntPtr(numWorkers),
			DB:         &dbOpts,
		}
		if namedQueue.Name != "" {
			perQueueOpts[namedQueue.Name] = queueOpts
			continue
		}

		re, err := regexp.Compile(namedQueue.Regexp)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "invalid regexp '%s'", namedQueue.Regexp)
		}
		regexpQueueOpts = append(regexpQueueOpts, queue.RegexpMongoDBQueueOptions{
			Regexp:  *re,
			Options: queueOpts,
		})
	}

	return perQueueOpts, regexpQueueOpts, nil
}

func (e *envState) createNotificationQueue(ctx context.Context) error {
	// Notifications queue w/ moving weight avg pool
	e.notificationsQueue = queue.NewLocalLimitedSize(len(e.senders), e.settings.Amboy.LocalStorage)

	runner, err := pool.NewMovingAverageRateLimitedWorkers(e.settings.Amboy.PoolSizeLocal,
		e.settings.Notify.BufferTargetPerInterval,
		time.Duration(e.settings.Notify.BufferIntervalSeconds)*time.Second,
		e.notificationsQueue)
	if err != nil {
		return errors.Wrap(err, "creating notifications queue")
	}
	if err = e.notificationsQueue.SetRunner(runner); err != nil {
		return errors.Wrap(err, "setting notifications queue runner")
	}
	rootSenders := []send.Sender{}
	for _, s := range e.senders {
		rootSenders = append(rootSenders, s)
	}

	e.RegisterCloser("notification-queue", false, func(ctx context.Context) error {
		var cancel context.CancelFunc
		catcher := grip.NewBasicCatcher()
		ctx, cancel = context.WithTimeout(ctx, queueShutdownWaitTimeout)
		defer cancel()
		if !amboy.WaitInterval(ctx, e.notificationsQueue, queueShutdownWaitInterval) {
			grip.Critical(message.Fields{
				"message": "pending jobs failed to finish",
				"queue":   "notifications",
				"status":  e.notificationsQueue.Stats(ctx),
			})
			catcher.New("failed to stop with running jobs")
		}

		e.notificationsQueue.Close(ctx)

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
	})

	for k := range e.senders {
		e.senders[k] = logger.MakeQueueSender(ctx, e.notificationsQueue, e.senders[k])
	}

	return nil
}

func (e *envState) initQueues(ctx context.Context) []error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(e.localQueue == nil, "local queue is not defined")
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

	e.clientConfig, err = getClientConfig(e.settings.Ui.Url, e.settings.HostInit.S3BaseURL)
	if err != nil {
		grip.Critical(message.WrapError(err, message.Fields{
			"message": "problem finding local clients",
			"cause":   "infrastructure configuration issue",
		}))
	} else if len(e.clientConfig.ClientBinaries) == 0 {
		grip.Critical("no clients binaries are available for this server")
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

	if e.settings.Notify.SES.SenderAddress != "" {
		config, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(DefaultEC2Region),
		)
		if err != nil {
			return errors.Wrap(err, "loading AWS config")
		}
		otelaws.AppendMiddlewares(&config.APIOptions)
		sesSender, err := send.NewSESLogger(ctx,
			send.SESOptions{
				Name:          "evergreen",
				AWSConfig:     config,
				SenderAddress: e.settings.Notify.SES.SenderAddress,
			}, levelInfo)
		if err != nil {
			return errors.Wrap(err, "setting up email logger")
		}
		e.senders[SenderEmail] = sesSender
	}

	// TODO EVG-19966: Remove global GitHub status sender
	var sender send.Sender
	githubToken, err := e.settings.GetGithubOauthToken()
	if err == nil && len(githubToken) > 0 {
		// Github Status
		sender, err = send.NewGithubStatusLogger("evergreen", &send.GithubOptions{
			Token:       githubToken,
			MinDelay:    GithubRetryMinDelay,
			MaxAttempts: GitHubRetryAttempts,
		}, "")
		if err != nil {
			return errors.Wrap(err, "setting up GitHub status logger")
		}
		e.senders[SenderGithubStatus] = sender
	}
	e.githubSenders = make(map[string]cachedGitHubSender)

	if jira := &e.settings.Jira; len(jira.GetHostURL()) != 0 {
		sender, err = send.NewJiraLogger(ctx, jira.Export(), levelInfo)
		if err != nil {
			return errors.Wrap(err, "setting up Jira issue logger")
		}
		e.senders[SenderJIRAIssue] = sender

		sender, err = send.NewJiraCommentLogger(ctx, "", jira.Export(), levelInfo)
		if err != nil {
			return errors.Wrap(err, "setting up Jira comment logger")
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
		}, slack.Token, levelInfo)
		if err != nil {
			return errors.Wrap(err, "setting up Slack logger")
		}
		e.senders[SenderSlack] = sender
	}

	sender, err = util.NewEvergreenWebhookLogger()
	if err != nil {
		return errors.Wrap(err, "setting up Evergreen webhook logger")
	}
	e.senders[SenderEvergreenWebhook] = sender

	sender, err = send.NewGenericLogger("evergreen", levelInfo)
	if err != nil {
		return errors.Wrap(err, "setting up Evergreen generic logger")
	}
	e.senders[SenderGeneric] = sender

	catcher := grip.NewBasicCatcher()
	for name, s := range e.senders {
		catcher.Add(s.SetLevel(levelInfo))
		tempName := name
		catcher.Add(s.SetErrorHandler(func(err error, m message.Composer) {
			if err == nil {
				return
			}
			grip.Error(message.WrapError(err, message.Fields{
				"notification":        m.String(),
				"message_type":        fmt.Sprintf("%T", m),
				"notification_target": tempName.String(),
				"event":               m,
			}))
		}))
	}
	return catcher.Resolve()
}

func (e *envState) initJasper() error {
	jpm, err := jasper.NewSynchronizedManager(true)
	if err != nil {
		return errors.WithStack(err)
	}

	e.jasperManager = jpm

	e.RegisterCloser("jasper-manager", true, func(ctx context.Context) error {
		return errors.WithStack(jpm.Close(ctx))
	})

	return nil
}

func (e *envState) initDepot(ctx context.Context) error {
	if e.settings.DomainName == "" {
		return errors.Errorf("bootstrapping collection '%s' requires domain name to be set in admin settings", CredentialsCollection)
	}

	maxExpiration := time.Duration(math.MaxInt64)

	bootstrapConfig := certdepot.BootstrapDepotConfig{
		CAName: CAName,
		MongoDepot: &certdepot.MongoDBOptions{
			MongoDBURI:     e.settings.Database.Url,
			DatabaseName:   e.settings.Database.DB,
			CollectionName: CredentialsCollection,
			DepotOptions: certdepot.DepotOptions{
				CA:                CAName,
				DefaultExpiration: maxExpiration,
			},
		},
		CAOpts: &certdepot.CertificateOptions{
			CA:         CAName,
			CommonName: CAName,
			Expires:    maxExpiration,
		},
		ServiceName: e.settings.DomainName,
		ServiceOpts: &certdepot.CertificateOptions{
			CA:         CAName,
			CommonName: e.settings.DomainName,
			Host:       e.settings.DomainName,
			Expires:    maxExpiration,
		},
	}

	var err error
	if e.depot, err = certdepot.BootstrapDepotWithMongoClient(ctx, e.client, bootstrapConfig); err != nil {
		return errors.Wrapf(err, "bootstrapping collection '%s'", CredentialsCollection)
	}

	return nil
}

func (e *envState) initTracer(ctx context.Context) error {
	if !e.settings.Tracer.Enabled {
		return nil
	}

	resource, err := resource.New(ctx,
		resource.WithHost(),
		resource.WithAttributes(semconv.ServiceName("evergreen")),
		resource.WithAttributes(semconv.ServiceVersion(BuildRevision)),
	)
	if err != nil {
		return errors.Wrap(err, "making otel resource")
	}

	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(e.settings.Tracer.CollectorEndpoint),
	)
	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		return errors.Wrap(err, "initializing otel exporter")
	}

	spanLimits := trace.NewSpanLimits()
	spanLimits.AttributeValueLengthLimit = OtelAttributeMaxLength

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(resource),
		trace.WithRawSpanLimits(spanLimits),
	)
	tp.RegisterSpanProcessor(utility.NewAttributeSpanProcessor())
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		grip.Error(errors.Wrap(err, "otel error"))
	}))

	e.RegisterCloser("otel-tracer-provider", false, func(ctx context.Context) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(tp.Shutdown(ctx))
		catcher.Add(exp.Shutdown(ctx))
		return nil
	})

	return nil
}

func (e *envState) setupRoleManager() error {
	e.roleManager = rolemanager.NewMongoBackedRoleManager(rolemanager.MongoBackedRoleManagerOpts{
		Client:          e.client,
		DBName:          e.dbName,
		RoleCollection:  RoleCollection,
		ScopeCollection: ScopeCollection,
	})

	catcher := grip.NewBasicCatcher()
	catcher.Add(e.roleManager.RegisterPermissions(ProjectPermissions))
	catcher.Add(e.roleManager.RegisterPermissions(DistroPermissions))
	catcher.Add(e.roleManager.RegisterPermissions(SuperuserPermissions))
	return catcher.Resolve()
}

func (e *envState) UserManager() gimlet.UserManager {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.userManager
}

func (e *envState) SetUserManager(um gimlet.UserManager) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.userManager = um
}

func (e *envState) UserManagerInfo() UserManagerInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.userManagerInfo
}

func (e *envState) SetUserManagerInfo(umi UserManagerInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.userManagerInfo = umi
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

type BuildBaronSettings struct {
	// todo: reconfigure the BuildBaronConfigured check to use TicketSearchProjects instead

	TicketCreateProject   string   `mapstructure:"ticket_create_project" bson:"ticket_create_project" json:"ticket_create_project" yaml:"ticket_create_project"`
	TicketCreateIssueType string   `mapstructure:"ticket_create_issue_type" bson:"ticket_create_issue_type" json:"ticket_create_issue_type" yaml:"ticket_create_issue_type"`
	TicketSearchProjects  []string `mapstructure:"ticket_search_projects" bson:"ticket_search_projects" json:"ticket_search_projects" yaml:"ticket_search_projects"`

	// The BF Suggestion server as a source of suggestions is only enabled for projects where BFSuggestionServer isn't the empty string.
	BFSuggestionServer      string `mapstructure:"bf_suggestion_server" bson:"bf_suggestion_server" json:"bf_suggestion_server" yaml:"bf_suggestion_server"`
	BFSuggestionUsername    string `mapstructure:"bf_suggestion_username" bson:"bf_suggestion_username" json:"bf_suggestion_username" yaml:"bf_suggestion_username"`
	BFSuggestionPassword    string `mapstructure:"bf_suggestion_password" bson:"bf_suggestion_password" json:"bf_suggestion_password" yaml:"bf_suggestion_password"`
	BFSuggestionTimeoutSecs int    `mapstructure:"bf_suggestion_timeout_secs" bson:"bf_suggestion_timeout_secs" json:"bf_suggestion_timeout_secs" yaml:"bf_suggestion_timeout_secs"`
	BFSuggestionFeaturesURL string `mapstructure:"bf_suggestion_features_url" bson:"bf_suggestion_features_url" json:"bf_suggestion_features_url" yaml:"bf_suggestion_features_url"`
}

type AnnotationsSettings struct {
	// a list of jira fields the user wants to display in addition to state assignee and priority
	JiraCustomFields []JiraField `mapstructure:"jira_custom_fields" bson:"jira_custom_fields" json:"jira_custom_fields" yaml:"jira_custom_fields"`
	// the endpoint that the user would like to send data to when the file ticket button is clicked
	FileTicketWebhook WebHook `mapstructure:"web_hook" bson:"web_hook" json:"web_hook" yaml:"file_ticket_webhook"`
}

type JiraField struct {
	// the name that jira calls the field
	Field string `mapstructure:"field" bson:"field" json:"field" yaml:"field"`
	// the name the user would like to call it in the UI
	DisplayText string `mapstructure:"display_text" bson:"display_text" json:"display_text" yaml:"display_text"`
}

type WebHook struct {
	Endpoint string `mapstructure:"endpoint" bson:"endpoint" json:"endpoint" yaml:"endpoint"`
	Secret   string `mapstructure:"secret" bson:"secret" json:"secret" yaml:"secret"`
}

func (e *envState) SaveConfig(ctx context.Context) error {
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
		return errors.Wrap(err, "copying settings")
	}

	gob.Register(map[interface{}]interface{}{})
	for pluginName, plugin := range copy.Plugins {
		if pluginName == "buildbaron" {
			for fieldName, field := range plugin {
				if fieldName == "projects" {
					var projects map[string]BuildBaronSettings
					err := mapstructure.Decode(field, &projects)
					if err != nil {
						return errors.Wrap(err, "decoding buildbaron projects")
					}
					plugin[fieldName] = projects
				}
			}
		}
	}

	return errors.WithStack(UpdateConfig(ctx, &copy))
}

// GetGitHubSender returns a cached sender with a GitHub app generated token. Each org in GitHub needs a separate token
// for authentication so we cache a sender for each org and return it if the token has not expired.
// If the sender for the org doesn't exist or has expired, we create a new one and cache it.
// In case of GitHub app errors, the function returns the legacy GitHub sender with a global token attached.
// The senders are only unique to orgs, not repos, but the repo name is needed to generate a token if necessary.
func (e *envState) GetGitHubSender(owner, repo string) (send.Sender, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	githubSender, ok := e.githubSenders[owner]
	// If githubSender exists and has not expired, return it.
	if ok && time.Since(githubSender.time) < githubTokenTimeout {
		return githubSender.sender, nil
	}
	// If githubSender does not exist or has expired, create one, add it to the cache, then return it.
	token, err := e.settings.CreateInstallationToken(e.ctx, owner, repo, nil)
	if err != nil {
		// TODO EVG-19966: Delete fallback to legacy GitHub sender
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "error creating installation token for GitHub sender",
			"owner":   owner,
			"repo":    repo,
			"ticket":  "EVG-19966",
		}))
		legacySender, ok := e.senders[SenderGithubStatus]
		if !ok {
			return nil, errors.Errorf("Legacy GitHub status sender not found")
		}
		return legacySender, nil
	}
	sender, err := send.NewGithubStatusLogger("evergreen", &send.GithubOptions{
		Token:       token,
		MinDelay:    GithubRetryMinDelay,
		MaxAttempts: GitHubRetryAttempts,
	}, "")
	if err != nil {
		// TODO EVG-19966: Delete fallback to legacy GitHub sender
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "error setting up GitHub status logger with GitHub app",
			"owner":   owner,
			"repo":    repo,
			"ticket":  "EVG-19966",
		}))
		legacySender, ok := e.senders[SenderGithubStatus]
		if !ok {
			return nil, errors.Errorf("Legacy GitHub status sender not found")
		}
		return legacySender, nil
	}
	e.githubSenders[owner] = cachedGitHubSender{
		sender: sender,
		time:   time.Now(),
	}
	return sender, nil
}

func (e *envState) GetSender(key SenderKey) (send.Sender, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	sender, ok := e.senders[key]
	if !ok {
		return nil, errors.Errorf("unknown sender key '%s'", key.String())
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

func (e *envState) RegisterCloser(name string, background bool, closer func(context.Context) error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.closers = append(e.closers, closerOp{name: name, background: background, closerFn: closer})
}

func (e *envState) Close(ctx context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	deadline, _ := ctx.Deadline()
	catcher := grip.NewBasicCatcher()
	wg := &sync.WaitGroup{}
	for n, closer := range e.closers {
		if !closer.background {
			continue
		}

		if closer.closerFn == nil {
			continue
		}

		wg.Add(1)
		go func(idx int, name string, clfn func(context.Context) error) {
			defer wg.Done()
			grip.Info(message.Fields{
				"message":      "calling closer",
				"index":        idx,
				"closer":       name,
				"timeout_secs": time.Until(deadline),
				"deadline":     deadline,
				"background":   true,
			})
			catcher.Add(clfn(ctx))
		}(n, closer.name, closer.closerFn)
	}

	for idx, closer := range e.closers {
		if closer.background {
			continue
		}
		if closer.closerFn == nil {
			continue
		}

		grip.Info(message.Fields{
			"message":      "calling closer",
			"index":        idx,
			"closer":       closer.name,
			"timeout_secs": time.Until(deadline),
			"deadline":     deadline,
			"background":   false,
		})
		catcher.Add(closer.closerFn(ctx))
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
func getClientConfig(baseURL, s3BaseURL string) (*ClientConfig, error) {
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
		displayName := ValidArchDisplayNames[fmt.Sprintf("%s_%s", buildInfo[0], buildInfo[1])]
		archPath := strings.Join(parts[len(parts)-2:], "/")
		c.ClientBinaries = append(c.ClientBinaries, ClientBinary{
			URL:         fmt.Sprintf("%s/%s/%s", baseURL, ClientDirectory, archPath),
			OS:          buildInfo[0],
			Arch:        buildInfo[1],
			DisplayName: displayName,
		})
		if s3BaseURL != "" {
			c.S3ClientBinaries = append(c.S3ClientBinaries, ClientBinary{
				URL: strings.Join([]string{
					strings.TrimSuffix(s3BaseURL, "/"),
					BuildRevision,
					archPath,
				}, "/"),
				OS:          buildInfo[0],
				Arch:        buildInfo[1],
				DisplayName: displayName,
			})
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "finding client binaries")
	}

	return c, nil
}

func (e *envState) JasperManager() jasper.Manager {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.jasperManager
}

func (e *envState) CertificateDepot() certdepot.Depot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.depot
}

func (e *envState) RoleManager() gimlet.RoleManager {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.roleManager
}
