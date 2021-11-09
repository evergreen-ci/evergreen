// Package poplar provides a set of tools for running and managing
// results for benchmarks in go.
//
//
// Tools and Infrastructure
//
// The Report type defines infrastructure that any project can use to
// create and upload test results to a cedar data service regardless
// of the execution model without needing to write client code that
// interacts with cedars RPC protocol.
//
// Additionally, poplar defines some-local only RPC interfaces on top
// of the github.com/mongodb/ftdc and github.com/mongodb/ftdc/events
// packages to support generating data payloads in real time.
//
//
// Report
//
// A Report structure structure, in YAML, would generally resemble the
// following:
//
//     project: <evergreen-project>
//     version: <evergreen-version>
//     variant: <evergreen-buildvariant>
//     task_name: <short evergreen task display name>
//     task_id: <unique evergreen task_id>
//     execution_number: <evergreen execution number>
//
//     bucket:
//       api_key: <aws api key>
//       api_secret: <aws api secret>
//       api_tokent: <aws api token>
//       region: <aws-region>
//
//       name: <bucket name>
//       prefix: <key prefix>
//
//     tests:
//       - info:
//           test_name: <local test name>
//           trial: <integer for repeated execution>
//           tags: [ "canary", <arbitrary>, <string>, <metadata> ]
//           arguments:
//              count: 1
//              iterations: 1000
//              # arbitrary test settings
//           create_at: <timestamp>
//           completed_at: <timestamp>
//           artifacts:
//              - bucket: <name>
//                path: <test>/local_data/client_data.ftdc
//                tags: [ "combined", "client" ]
//                is_ftdc: true
//                events_raw: true
//                created_at: <timestamp>
//                local_file: path/to/client_data.ftdc
//              - bucket: <name>
//                path: <test>/local_data/client_data.ftdc
//                tags: [ "combined", "server" ]
//                is_ftdc: true
//                created_at: <timestamp>
//                local_file: path/to/server_data.ftdc
//           test: [] # subtests with the same test structure
//
// See the documentation of the Report, Test, TestInfo, TestArtifact
// and TestMetrics format for more information.
//
//
// Benchmarks
//
// Poplar contains a toolkit for running benchmark tests and
// collecting rich intra-run data from those tests. Consider the
// following example:
//
//	suite := BenchmarkSuite{
//		{
//			CaseName: "HelloWorld",
//			Bench: func(ctx context.Context, r events.Recorder, count int) error {
//				out := []string{}
//				for i := 0; i < count; i++ {
//					startAt := time.Now()
//					r.Begin()
//					val := "Hello World"
//					out = append(out, val)
//					r.End(time.Since(startAt))
//					r.IncOps(1)
//					r.IncSize(len(val))
//				}
//				return nil
//			},
//			MinRuntime:    5 * time.Second,
//			MaxRuntime:    time.Minute,
//			MinIterations: 1000000,
//			Count:         1000,
//			Recorder:      poplar.RecorderPerf,
//		},
//	}
//
//	results := suite.Run(ctx, "poplar_raw")
//	grip.Info(results.Composer()) // log results.
//
//         // Optionally send results to cedar
//
//	report := &Report{
//		Project:  "poplar test",
//		Version:  "<evg-version>",
//		Variant:  "arch-linux",
//		TaskName: "perf",
//		TaskID:   "<evg task>",
//		BucketConf: BucketConfiguration{
//			APIKey:    "<key>",
//			APISecret: "<secret>",
//			APIToken:  "token",
//			Bucket:    "poplarresults",
//			Prefix:    "perf",
//		},
//		Tests: results.Export(),
//	}
//
//
//	var cc *grpc.ClientConn // set up service connection to cedar
//	err := rpc.Upload(ctx, report, cc)
//	if err != nil {
//		grip.EmergencyFatal(err) // exit
//	}
//
// You can also run a benchmark suite using go's standard library, as
// in:
//
//      registry := NewRegistry() // create recorder recorder infrastructure
//
//      func BenchmarkHelloWorldSuite(b *testing.B) {
//		suite.Standard(registry)(b)
//      }
//
// Each test in the suite is reported as a separate sub-benchmark.
//
//
// Workloads
//
// In addition to suites, workloads provide a way to define
// concurrent and parallel workloads that execute multiple instances
// of a single test at a time. Workloads function like an extended
// case of suites.
//
//	workload := &BenchmarkWorkload{
//		Name:      "HelloWorld",
//		Instances: 10,
//		Recorder:  poplar.RecorderPerf,
//		Case:      &BenchmarkCase{},
//	}
//
// You must specify either a single case or a list of
// sub-workloads. The executors for workloads run the groups of these
// tests in parallel, with the degree of parallelism controlled by the
// Instances value. When you specify a list of sub-workloads, poplar
// will execute a group of these workloads in parallel, but the
// workloads themselves are run sequentially, potentially with their
// own parallelism.
//
// Benchmark suites and workloads both report data in the same
// format. You can also execute workloads using the Standard method,
// as with suites, using default Go methods for running benchmarks.
package poplar

// This file is intentionally documentation only.
