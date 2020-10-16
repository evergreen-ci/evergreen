package internal

import (
	"github.com/evergreen-ci/poplar"
	"google.golang.org/grpc"
)

func AttachService(registry *poplar.RecorderRegistry, s *grpc.Server) error {
	RegisterPoplarMetricsRecorderServer(s, &recorderService{
		registry: registry,
	})
	RegisterPoplarMetricsCollectorServer(s, &metricsService{
		registry: registry,
	})
	RegisterPoplarEventCollectorServer(s, &collectorService{
		registry: registry,
		coordinator: &streamsCoordinator{
			groups: map[string]*streamGroup{},
		},
	})

	return nil
}
