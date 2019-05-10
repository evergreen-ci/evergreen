package jasper

import (
	"context"
	"fmt"
	"net/http"
	"syscall"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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
			grip.Warning("timed out starting test service service")
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
					fmt.Println("HAPPENS")
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

////////////////////////////////////////////////////////////////////////
//
// mock of manager with configurable failures

type MockManager struct {
	FailCreate   bool
	FailRegister bool
	FailList     bool
	FailGroup    bool
	FailGet      bool
	FailClear    bool
	FailClose    bool
	Process      *MockProcess
	Array        []Process
}

func (m *MockManager) CreateProcess(_ context.Context, opts *CreateOptions) (Process, error) {
	if m.FailCreate {
		return nil, errors.New("always fail")
	}

	return m.Process, nil
}

func (m *MockManager) CreateCommand(_ context.Context) *Command {
	return NewCommand().ProcConstructor(m.CreateProcess)
}

func (m *MockManager) Register(_ context.Context, proc Process) error {
	if m.FailRegister {
		return errors.New("always fail")
	}
	return nil
}

func (m *MockManager) List(_ context.Context, f Filter) ([]Process, error) {
	if m.FailList {
		return nil, errors.New("always fail")
	}
	return m.Array, nil
}

func (m *MockManager) Group(_ context.Context, name string) ([]Process, error) {
	if m.FailGroup {
		return nil, errors.New("always fail")
	}

	return m.Array, nil
}

func (m *MockManager) Get(_ context.Context, name string) (Process, error) {
	if m.FailGet {
		return nil, errors.New("always fail")
	}

	return m.Process, nil
}

func (m *MockManager) Clear(_ context.Context) {
	return
}

func (m *MockManager) Close(_ context.Context) error {
	if m.FailClose {
		return errors.New("always fail")
	}
	return nil
}

type MockProcess struct {
	ProcID                    string
	ProcInfo                  ProcessInfo
	IsRunning                 bool
	IsComplete                bool
	FailSignal                bool
	FailWait                  bool
	FailRespawn               bool
	FailRegisterTrigger       bool
	FailRegisterSignalTrigger bool
}

func (p *MockProcess) ID() string                         { return p.ProcID }
func (p *MockProcess) Info(_ context.Context) ProcessInfo { return p.ProcInfo }
func (p *MockProcess) Running(_ context.Context) bool     { return p.IsRunning }
func (p *MockProcess) Complete(_ context.Context) bool    { return p.IsComplete }
func (p *MockProcess) GetTags() []string                  { return nil }
func (p *MockProcess) Tag(s string)                       {}
func (p *MockProcess) ResetTags()                         {}
func (p *MockProcess) Signal(_ context.Context, s syscall.Signal) error {
	if p.FailSignal {
		return errors.New("always fail")
	}

	return nil
}

func (p *MockProcess) Wait(_ context.Context) (int, error) {
	if p.FailWait {
		return -1, errors.New("always fail")
	}

	return 0, nil
}

func (p *MockProcess) Respawn(_ context.Context) (Process, error) {
	if p.FailRespawn {
		return nil, errors.New("always fail")
	}

	return nil, nil
}

func (p *MockProcess) RegisterTrigger(_ context.Context, t ProcessTrigger) error {
	if p.FailRegisterTrigger {
		return errors.New("always fail")
	}

	return nil
}

func (p *MockProcess) RegisterSignalTrigger(_ context.Context, _ SignalTrigger) error {
	if p.FailRegisterSignalTrigger {
		return errors.New("always fail")
	}

	return nil
}

func (p *MockProcess) RegisterSignalTriggerID(_ context.Context, _ SignalTriggerID) error {
	if p.FailRegisterSignalTrigger {
		return errors.New("always fail")
	}

	return nil
}
