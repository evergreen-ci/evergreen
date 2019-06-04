package rest

import (
	"context"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// QueueService is used as a place holder for application state and configuration.
type QueueService struct {
	queue           amboy.Queue
	closer          context.CancelFunc
	registeredTypes []string
}

// NewQueueService constructs a new service object. Use the Open() method
// to initialize the service. The Open and OpenWithOptions methods
// configure an embedded amboy service. If you use SetQueue you do not
// need to call an open method.
//
// Use the App() method to get a gimlet Application that you can use
// to run the service.
func NewQueueService() *QueueService {
	service := &QueueService{}

	for name := range registry.JobTypeNames() {
		service.registeredTypes = append(service.registeredTypes, name)
	}

	return service
}

// App provides access to the gimplet.APIApp instance which builds and
// orchestrates the REST API. Use this method if you want to combine
// the routes in this QueueService with another service, or add additional
// routes to support other application functionality.
func (s *QueueService) App() *gimlet.APIApp {
	app := gimlet.NewApp()

	app.AddRoute("/").Version(0).Get().Handler(s.Status)
	app.AddRoute("/status").Version(1).Get().Handler(s.Status)
	app.AddRoute("/status/wait").Version(1).Get().Handler(s.WaitAll)
	app.AddRoute("/job/create").Version(1).Post().Handler(s.Create)
	app.AddRoute("/job/{name}").Version(1).Get().Handler(s.Fetch)
	app.AddRoute("/job/status/{name}").Version(1).Get().Handler(s.JobStatus)
	app.AddRoute("/job/wait/{name}").Version(1).Get().Handler(s.WaitJob)

	return app
}

// Open populates the application and starts the underlying
// queue. This method sets and initializes a LocalLimitedSize queue
// implementation, with 2 workers and storage for 256 jobs. Use
// OpenInfo to have more control over the embedded queue. Use the
// Close() method on the service to terminate the queue.
func (s *QueueService) Open(ctx context.Context) error {
	opts := QueueServiceOptions{
		ForceTimeout: time.Duration(0),
		QueueSize:    256,
		NumWorkers:   2,
	}

	if err := s.OpenWithOptions(ctx, opts); err != nil {
		return errors.Wrap(err, "could not open queue.")
	}

	return nil
}

// QueueServiceOptions provides a way to configure resources allocated by a service.
type QueueServiceOptions struct {
	// ForceTimeout makes it possible to control how long to wait
	// for pending jobs to complete before canceling existing
	// work. If this value is zeroed, the Open operation will
	// close the previous queue and start a new queue, otherwise
	// it will attempt to wait for the specified time before closing
	// the previous queue.
	ForceTimeout time.Duration `bson:"force_timeout,omitempty" json:"force_timeout,omitempty" yaml:"force_timeout,omitempty"`

	// The default queue constructed by Open/OpenWith retains a
	// limited number of completed jobs to avoid unbounded memory
	// growth. This value *must* be specified.
	QueueSize int `bson:"queue_size" json:"queue_size" yaml:"queue_size"`

	// Controls the maximum number of go routines that can service
	// jobs in a queue.
	NumWorkers int `bson:"num_workers" json:"num_workers" yaml:"num_workers"`
}

// OpenWithOptions makes it possible to configure the underlying queue in a
// service. Use the Close() method on the service to terminate the queue.
func (s *QueueService) OpenWithOptions(ctx context.Context, opts QueueServiceOptions) error {
	if opts.NumWorkers == 0 || opts.QueueSize == 0 {
		return errors.Errorf("cannot build service with specified options: %+v", opts)
	}

	if s.closer != nil {
		if opts.ForceTimeout != 0 {
			waiterCtx, cancel := context.WithTimeout(ctx, opts.ForceTimeout)
			grip.Info("waiting for jobs to complete")
			amboy.Wait(waiterCtx, s.queue)
			cancel()
		}
		grip.Info("releasing remaining queue resources")
		s.closer()
	}

	ctx, cancel := context.WithCancel(ctx)
	s.closer = cancel

	s.queue = queue.NewLocalLimitedSize(opts.NumWorkers, opts.QueueSize)
	grip.Alert(s.queue.Start(ctx))

	return nil
}

// Close releases resources (i.e. the queue) associated with the
// service. If you've used SetQueue to define the embedded queue,
// rather than Open/OpenWithOptions,
func (s *QueueService) Close() {
	if s.closer != nil {
		s.closer()
	}
}

// Queue provides access to the underlying queue object for the service.
func (s *QueueService) Queue() amboy.Queue {
	return s.queue
}

// SetQueue allows callers to inject an alternate queue implementation.
func (s *QueueService) SetQueue(q amboy.Queue) error {
	if s.closer != nil {
		return errors.New("cannot set a new queue, QueueService is already open")
	}

	s.queue = q
	return nil
}
