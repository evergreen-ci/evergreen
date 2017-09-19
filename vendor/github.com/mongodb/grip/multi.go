/*
Error Catcher

The MutiCatcher type makes it possible to collect from a group of
operations and then aggregate them as a single error.
*/
package grip

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// TODO: make a new catcher package, leave constructors in this
// package, use this Catcher interface, and write implementations that
// don't translate errors into string internally.
//

// Catcher is an interface for an error collector for use when
// implementing continue-on-error semantics in concurrent
// operations. There are three different Catcher implementations
// provided by this package that differ *only* in terms of the
// string format returned by String() (and also the format of the
// error returned by Resolve().)
//
// If you do not use github.com/pkg/errors to attach
// errors, the implementations are usually functionally
// equivalent. The Extended variant formats the errors using the "%+v"
// (which returns a full stack trace with pkg/errors,) the Simple
// variant uses %s (which includes all the wrapped context,) and the
// basic catcher calls error.Error() (which should be equvalent to %s
// for most error implementations.)
type Catcher interface {
	Add(error)
	Len() int
	HasErrors() bool
	String() string
	Resolve() error
}

// multiCatcher provides an interface to collect and coalesse error
// messages within a function or other sequence of operations. Used to
// implement a kind of "continue on error"-style operations. The
// methods on MultiCatatcher are thread-safe.
type baseCatcher struct {
	errs  []error
	mutex sync.RWMutex
	fmt.Stringer
}

// NewCatcher returns a Catcher instance that you can use to capture
// error messages and aggregate the errors. For consistency with
// earlier versions NewCatcher is the same as NewExtendedCatcher()
//
// DEPRECATED: use one of the other catcher implementations. See the
// documentation for the Catcher interface for most implementations.
func NewCatcher() Catcher { return NewExtendedCatcher() }

func NewBasicCatcher() Catcher {
	c := &baseCatcher{}
	c.Stringer = &basicCatcher{c}
	return c
}

func NewSimpleCatcher() Catcher {
	c := &baseCatcher{}
	c.Stringer = &simpleCatcher{c}
	return c
}

func NewExtendedCatcher() Catcher {
	c := &baseCatcher{}
	c.Stringer = &extendedCatcher{c}
	return c
}

// Add takes an error object and, if it's non-nil, adds it to the
// internal collection of errors.
func (c *baseCatcher) Add(err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err != nil {
		c.errs = append(c.errs, err)
	}
}

// Len returns the number of errors stored in the collector.
func (c *baseCatcher) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.errs)
}

// HasErrors returns true if the collector has ingested errors, and
// false otherwise.
func (c *baseCatcher) HasErrors() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.errs) > 0
}

// Resolve returns a final error object for the Catcher. If there are
// no errors, it returns nil, and returns an error object with the
// string form of all error objects in the collector.
func (c *baseCatcher) Resolve() error {
	if !c.HasErrors() {
		return nil
	}

	return errors.New(c.String())
}

////////////////////////////////////////////////////////////////////////
//
// seperate implementations of grip.Catcher with different string formatting options.

type extendedCatcher struct{ *baseCatcher }

func (c *extendedCatcher) String() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	output := make([]string, len(c.errs))

	for idx, err := range c.errs {
		output[idx] = fmt.Sprintf("%+v", err)
	}

	return strings.Join(output, "\n")
}

type simpleCatcher struct{ *baseCatcher }

func (c *simpleCatcher) String() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	output := make([]string, len(c.errs))

	for idx, err := range c.errs {
		output[idx] = fmt.Sprintf("%s", err)
	}

	return strings.Join(output, "\n")
}

type basicCatcher struct{ *baseCatcher }

func (c *basicCatcher) String() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	output := make([]string, len(c.errs))

	for idx, err := range c.errs {
		output[idx] = err.Error()
	}

	return strings.Join(output, "\n")
}
