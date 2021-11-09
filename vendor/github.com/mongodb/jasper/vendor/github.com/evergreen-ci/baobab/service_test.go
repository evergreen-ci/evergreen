// Copyright 2015 Daniel Theophanes.
// Use of this source code is governed by a zlib-style
// license that can be found in the LICENSE file.

package baobab_test

import (
	"testing"
	"time"

	"github.com/evergreen-ci/baobab"
)

func TestRunInterrupt(t *testing.T) {
	p := &program{}
	sc := &baobab.Config{
		Name: "go_baobab_test",
	}
	s, err := service.New(p, sc)
	if err != nil {
		t.Fatalf("New err: %s", err)
	}

	go func() {
		<-time.After(1 * time.Second)
		interruptProcess(t)
	}()

	go func() {
		for i := 0; i < 25 && p.numStopped == 0; i++ {
			<-time.After(200 * time.Millisecond)
		}
		if p.numStopped == 0 {
			t.Fatal("Run() hasn't been stopped")
		}
	}()

	if err = s.Run(); err != nil {
		t.Fatalf("Run() err: %s", err)
	}
}

type program struct {
	numStopped int
}

func (p *program) Start(s baobab.Service) error {
	go p.run()
	return nil
}
func (p *program) run() {
	// Do work here
}
func (p *program) Stop(s baobab.Service) error {
	p.numStopped++
	return nil
}
