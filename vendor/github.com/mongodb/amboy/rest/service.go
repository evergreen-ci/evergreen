package rest

import (
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
	"github.com/tychoish/gimlet"
	"github.com/mongodb/grip"
	"golang.org/x/net/context"
)

// Service is used as a place holder for application state and configuration.
type Service struct {
	queue           amboy.Queue
	closer          context.CancelFunc
	registeredTypes []string
	app             *gimlet.APIApp
}

// NewService constructs a new service object. Use the Open() method
// to initialize the service, and the Run() method to start the
// service. The Open and OpenInfo methods configure an embedded amboy
// Queue; if you use SetQueue you do not need to call
func NewService() *Service {
	service := &Service{}

	for name := range registry.JobTypeNames() {
		service.registeredTypes = append(service.registeredTypes, name)
	}

	return service
}

// Open populates the application and starts the underlying
// queue. This method sets and initializes a LocalLimitedSize queue
// implementation, with 2 workers and storage for 256 jobs. Use
// OpenInfo to have more control over the embedded queue. Use the
// Close() method on the service to terminate the queue.
func (s *Service) Open(ctx context.Context) error {
	info := ServiceInfo{time.Duration(0), 256, 2}

	if err := s.OpenInfo(ctx, info); err != nil {
		return errors.Wrap(err, "could not open queue.")
	}

	return nil
}

// ServiceInfo provides a way to configure resources allocated by a service.
type ServiceInfo struct {
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

// OpenInfo makes it possible to configure the underlying queue in a
// service. Use the Close() method on the service to terminate the queue.
func (s *Service) OpenInfo(ctx context.Context, info ServiceInfo) error {
	if info.NumWorkers == 0 || info.QueueSize == 0 {
		return errors.Errorf("cannot build service with specified information: %+v", info)
	}

	if s.closer != nil {
		if info.ForceTimeout != 0 {
			waiterCtx, cancel := context.WithTimeout(ctx, info.ForceTimeout)
			grip.Info("waiting for jobs to complete")
			amboy.WaitCtx(waiterCtx, s.queue)
			cancel()
		}
		grip.Info("releasing remaining queue resources")
		s.closer()
	}

	ctx, cancel := context.WithCancel(ctx)
	s.closer = cancel

	s.queue = queue.NewLocalLimitedSize(info.NumWorkers, info.QueueSize)
	grip.CatchAlert(s.queue.Start(ctx))

	return nil
}

// Close releases resources (i.e. the queue) associated with the
// service. If you've used SetQueue to define the embedded queue, rather than Open/OpenInfo,
func (s *Service) Close() {
	if s.closer != nil {
		s.closer()
	}
}

// Queue provides access to the underlying queue object for the service.
func (s *Service) Queue() amboy.Queue {
	return s.queue
}

// SetQueue allows callers to inject an alternate queue implementation.
func (s *Service) SetQueue(q amboy.Queue) error {
	if s.closer != nil {
		return errors.New("cannot set a new queue, Service is already open")
	}

	s.queue = q
	return nil
}

// App provides access to the gimplet.APIApp instance which builds and
// orchestrates the REST API. Use this method if you want to combine
// the routes in this Service with another service, or add additional
// routes to support other application functionality.
func (s *Service) App() *gimlet.APIApp {
	if s.app == nil {
		s.app = gimlet.NewApp()
		s.app.SetDefaultVersion(0)

		s.app.AddRoute("/").Version(0).Get().Handler(s.Status)
		s.app.AddRoute("/status").Version(1).Get().Handler(s.Status)
		s.app.AddRoute("/status/wait").Version(1).Get().Handler(s.WaitAll)
		s.app.AddRoute("/job/create").Version(1).Post().Handler(s.Create)
		s.app.AddRoute("/job/{name}").Version(1).Get().Handler(s.Fetch)
		s.app.AddRoute("/job/status/{name}").Version(1).Get().Handler(s.JobStatus)
		s.app.AddRoute("/job/wait/{name}").Version(1).Get().Handler(s.WaitJob)
	}

	return s.app
}
