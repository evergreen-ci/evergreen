/*
Error Catcher

The MutiCatcher type makes it possible to collect from a group of
operations and then aggregate them as a single error.
*/
package grip

import (
	"fmt"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

// TODO: make a new catcher package, leave constructors in this
// package, use this Catcher interface, and write implementations that
// don't translate errors into string internally.
//

// CheckFunction are functions which take no arguments and return an
// error.
type CheckFunction func() error

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
	AddWhen(bool, error)
	Extend([]error)
	ExtendWhen(bool, []error)
	Len() int
	HasErrors() bool
	String() string
	Resolve() error
	Errors() []error

	New(string)
	NewWhen(bool, string)
	Errorf(string, ...interface{})
	ErrorfWhen(bool, string, ...interface{})

	Wrap(error, string)
	Wrapf(error, string, ...interface{})

	Check(CheckFunction)
	CheckExtend([]CheckFunction)
	CheckWhen(bool, CheckFunction)
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

// NewBasicCatcher collects error messages and formats them using a
// new-line separated string of the output of error.Error()
func NewBasicCatcher() Catcher {
	c := &baseCatcher{}
	c.Stringer = &basicCatcher{c}
	return c
}

// NewSimpleCatcher collects error messages and formats them using a
// new-line separated string of the string format of the error message
// (e.g. %s).
func NewSimpleCatcher() Catcher {
	c := &baseCatcher{}
	c.Stringer = &simpleCatcher{c}
	return c
}

// NewExtendedCatcher collects error messages and formats them using a
// new-line separated string of the extended string format of the
// error message (e.g. %+v).
func NewExtendedCatcher() Catcher {
	c := &baseCatcher{}
	c.Stringer = &extendedCatcher{c}
	return c
}

// Add takes an error object and, if it's non-nil, adds it to the
// internal collection of errors.
func (c *baseCatcher) Add(err error) {
	if err == nil {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.errs = append(c.errs, err)
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

// Extend adds all non-nil errors, passed as arguments to the catcher.
func (c *baseCatcher) Extend(errs []error) {
	if len(errs) == 0 {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, err := range errs {
		if err == nil {
			continue
		}

		c.errs = append(c.errs, err)
	}
}

func (c *baseCatcher) Errorf(form string, args ...interface{}) {
	if form == "" {
		return
	} else if len(args) == 0 {
		c.New(form)
		return
	}
	c.Add(errors.Errorf(form, args...))
}

func (c *baseCatcher) New(e string) {
	if e == "" {
		return
	}
	c.Add(errors.New(e))
}

func (c *baseCatcher) Wrap(err error, m string) { c.Add(errors.Wrap(err, m)) }

func (c *baseCatcher) Wrapf(err error, f string, args ...interface{}) {
	c.Add(errors.Wrapf(err, f, args...))
}

func (c *baseCatcher) AddWhen(cond bool, err error) {
	if !cond {
		return
	}

	c.Add(err)
}

func (c *baseCatcher) ExtendWhen(cond bool, errs []error) {
	if !cond {
		return
	}

	c.Extend(errs)
}

func (c *baseCatcher) ErrorfWhen(cond bool, form string, args ...interface{}) {
	if !cond {
		return
	}

	c.Errorf(form, args...)
}

func (c *baseCatcher) NewWhen(cond bool, e string) {
	if !cond {
		return
	}

	c.New(e)
}

func (c *baseCatcher) Check(fn CheckFunction) { c.Add(fn()) }

func (c *baseCatcher) CheckWhen(cond bool, fn CheckFunction) {
	if !cond {
		return
	}

	c.Add(fn())
}

func (c *baseCatcher) CheckExtend(fns []CheckFunction) {
	for _, fn := range fns {
		c.Add(fn())
	}
}

func (c *baseCatcher) Errors() []error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	out := make([]error, len(c.errs))

	copy(out, c.errs)

	return out
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
// separate implementations of grip.Catcher with different string formatting options.

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

////////////////////////////////////////////////////////////////////////
//
// an implementation to annotate errors with timestamps

type timeAnnotatingCatcher struct {
	errs     []*timestampError
	extended bool
	mu       sync.RWMutex
}

// NewTimestampCatcher produces a Catcher instance that reports the
// short form of all constituent errors and annotates those errors
// with a timestamp to reflect when the error was collected.
func NewTimestampCatcher() Catcher { return &timeAnnotatingCatcher{} }

// NewExtendedTimestampCatcher adds long-form annotation to the
// aggregated error message (e.g. including stacks, when possible.)
func NewExtendedTimestampCatcher() Catcher { return &timeAnnotatingCatcher{extended: true} }

func (c *timeAnnotatingCatcher) Add(err error) {
	if err == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	tserr := newTimeStampError(err)

	if tserr == nil {
		return
	}

	c.errs = append(c.errs, tserr.setExtended(c.extended))
}

func (c *timeAnnotatingCatcher) Extend(errs []error) {
	if len(errs) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, err := range errs {
		if err == nil {
			continue
		}

		c.errs = append(c.errs, newTimeStampError(err).setExtended(c.extended))
	}
}

func (c *timeAnnotatingCatcher) AddWhen(cond bool, err error) {
	if !cond {
		return
	}

	c.Add(err)
}

func (c *timeAnnotatingCatcher) ExtendWhen(cond bool, errs []error) {
	if !cond {
		return
	}

	c.Extend(errs)
}

func (c *timeAnnotatingCatcher) New(e string) {
	if e == "" {
		return
	}

	c.Add(errors.New(e))
}

func (c *timeAnnotatingCatcher) NewWhen(cond bool, e string) {
	if !cond {
		return
	}

	c.New(e)
}

func (c *timeAnnotatingCatcher) Errorf(f string, args ...interface{}) {
	if f == "" {
		return
	} else if len(args) == 0 {
		c.New(f)
		return
	}

	c.Add(errors.Errorf(f, args...))
}

func (c *timeAnnotatingCatcher) ErrorfWhen(cond bool, f string, args ...interface{}) {
	if !cond {
		return
	}

	c.Errorf(f, args...)
}

func (c *timeAnnotatingCatcher) Wrap(err error, m string) {
	c.Add(WrapErrorTimeMessage(err, m))
}

func (c *timeAnnotatingCatcher) Wrapf(err error, f string, args ...interface{}) {
	c.Add(WrapErrorTimeMessagef(err, f, args...))
}

func (c *timeAnnotatingCatcher) Check(fn CheckFunction) {
	c.Add(fn())
}

func (c *timeAnnotatingCatcher) CheckWhen(cond bool, fn CheckFunction) {
	if !cond {
		return
	}

	c.Add(fn())
}

func (c *timeAnnotatingCatcher) CheckExtend(fns []CheckFunction) {
	for _, fn := range fns {
		c.Add(fn())
	}
}

func (c *timeAnnotatingCatcher) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.errs)
}

func (c *timeAnnotatingCatcher) HasErrors() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.errs) > 0
}

func (c *timeAnnotatingCatcher) Errors() []error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]error, len(c.errs))
	for idx, err := range c.errs {
		out[idx] = err
	}

	return out
}

func (c *timeAnnotatingCatcher) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	output := make([]string, len(c.errs))

	for idx, err := range c.errs {
		if err.extended {
			output[idx] = err.String()
		} else {
			output[idx] = err.String()
		}
	}

	return strings.Join(output, "\n")
}

func (c *timeAnnotatingCatcher) Resolve() error {
	if !c.HasErrors() {
		return nil
	}

	return errors.New(c.String())
}
