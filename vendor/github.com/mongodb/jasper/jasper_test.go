package jasper

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mongodb/grip"
)

var intSource <-chan int

func init() {
	intSource = func() <-chan int {
		out := make(chan int, 25)
		go func() {
			id := 3000
			for {
				id++
				out <- id
			}
		}()
		return out
	}()
}

func getPortNumber() int {
	return <-intSource

}

const (
	taskTimeout        = 5 * time.Second
	processTestTimeout = 15 * time.Second
	managerTestTimeout = 5 * taskTimeout
	longTaskTimeout    = 100 * time.Second
)

func makeLockingProcess(pmake ProcessConstructor) ProcessConstructor {
	return func(ctx context.Context, opts *CreateOptions) (Process, error) {
		proc, err := pmake(ctx, opts)
		if err != nil {
			return nil, err
		}
		return &localProcess{proc: proc}, nil
	}
}

// this file contains tools and constants used throughout the test
// suite.

func trueCreateOpts() *CreateOptions {
	return &CreateOptions{
		Args: []string{"true"},
	}
}

func falseCreateOpts() *CreateOptions {
	return &CreateOptions{
		Args: []string{"false"},
	}
}

func sleepCreateOpts(num int) *CreateOptions {
	return &CreateOptions{
		Args: []string{"sleep", fmt.Sprint(num)},
	}
}

func createProcs(ctx context.Context, opts *CreateOptions, manager Manager, num int) ([]Process, error) {
	catcher := grip.NewBasicCatcher()
	out := []Process{}
	for i := 0; i < num; i++ {
		optsCopy := *opts

		proc, err := manager.CreateProcess(ctx, &optsCopy)
		catcher.Add(err)
		if proc != nil {
			out = append(out, proc)
		}
	}

	return out, catcher.Resolve()
}

func makeAndStartService(ctx context.Context, client *http.Client) (*Service, int) {
outerRetry:
	for {
		select {
		case <-ctx.Done():
			grip.Warning("timed out starting test service")
			return nil, -1
		default:
			port := getPortNumber()
			localManager, err := NewLocalManager(false)
			if err != nil {
				return nil, -1
			}
			srv := NewManagerService(localManager)
			app := srv.App(ctx)
			app.SetPrefix("jasper")
			if err := app.SetPort(port); err != nil {
				continue outerRetry
			}
			go func() {
				app.Run(ctx)
			}()

			timer := time.NewTimer(5 * time.Millisecond)
			defer timer.Stop()
			url := fmt.Sprintf("http://localhost:%d/jasper/v1/", port)

			trials := 0
		checkLoop:
			for {
				if trials > 40 {
					continue outerRetry
				}

				select {
				case <-ctx.Done():
					return nil, -1
				case <-timer.C:
					req, err := http.NewRequest(http.MethodGet, url, nil)
					if err != nil {
						timer.Reset(5 * time.Millisecond)
						trials++
						continue checkLoop
					}
					req = req.WithContext(ctx)
					resp, err := client.Do(req)
					if err != nil {
						timer.Reset(5 * time.Millisecond)
						trials++
						continue checkLoop
					}
					if resp.StatusCode != http.StatusOK {
						timer.Reset(5 * time.Millisecond)
						trials++
						continue checkLoop
					}

					return srv, port
				}
			}
		}
	}
}
