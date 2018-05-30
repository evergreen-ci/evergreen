package gimlet

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

// Responder is an interface for constructing a response from a
// route. Fundamentally Responders are data types, and provide setters
// and getters to store data that
//
// In general, users will use one of gimlets response implementations,
// though clients may wish to build their own implementations to
// provide features not present in the existing
type Responder interface {
	// Validate returns an error if the page is not properly
	// constructed, although it is implementation specific, what
	// constitutes an invalid page.
	Validate() error

	// The data aspect of the interface holds the body of the
	// response. Implementations may handle multiple calls to
	// AddData differently, and provide different levels of
	// validation.
	Data() interface{}
	AddData(interface{}) error

	// Format represents the serialization format (and/or MIME
	// type) of the data payload on output. These options are
	// defined in gimlet, which supports JSON, YAML, Plain Text,
	// HTML, and Binary.
	Format() OutputFormat
	SetFormat(OutputFormat) error

	// Status returns the HTTP static code for this
	// responses. SetStatus implementations should not allow users
	// to set invalid statuses.
	Status() int
	SetStatus(int) error

	// The ResponsePage methods add and return the pagination
	// metdata for this route.
	//
	// Implementations should return nil pages to have an
	// unpaginated response.
	Pages() *ResponsePages
	SetPages(*ResponsePages) error
}

// NewResponseBuilder constructs a Responder implementation that can
// be used to incrementally build a with successive calls to AddData().
func NewResponseBuilder() Responder { return &responseBuilder{} }

type responseBuilder struct {
	data   []interface{}
	format OutputFormat
	status int
	pages  *ResponsePages
}

func (r *responseBuilder) Data() interface{} {
	switch len(r.data) {
	case 1:
		return r.data[0]
	case 0:
		return struct{}{}
	default:
		return r.data
	}
}

func (r *responseBuilder) Validate() error       { return nil }
func (r *responseBuilder) Format() OutputFormat  { return r.format }
func (r *responseBuilder) Status() int           { return r.status }
func (r *responseBuilder) Pages() *ResponsePages { return r.pages }

func (r *responseBuilder) AddData(d interface{}) error {
	if d == nil {
		return errors.New("cannot  data to responder")
	}

	r.data = append(r.data, d)
	return nil
}

func (r *responseBuilder) SetFormat(o OutputFormat) error {
	if !o.IsValid() {
		return errors.New("invalid output format")
	}

	r.format = o
	return nil
}

func (r *responseBuilder) SetStatus(s int) error {
	if http.StatusText(s) == "" {
		return fmt.Errorf("%d is not a valid HTTP status", s)
	}

	r.status = s
	return nil
}

func (r *responseBuilder) SetPages(p *ResponsePages) error {
	if err := p.Validate(); err != nil {
		return errors.Wrap(err, "cannot set an invalid page definition")
	}

	r.pages = p
	return nil
}

// NewBasicResponder constructs a Responder from the arguments passed
// to the constructor, though interface remains mutable.
//
// This implementation only allows a single data object, and AddData
// will overwrite existing data as set.
func NewBasicResponder(s int, f OutputFormat, data interface{}) (Responder, error) {
	r := &responderImpl{}

	errs := []string{}
	if err := r.SetStatus(s); err != nil {
		errs = append(errs, err.Error())
	}

	if err := r.AddData(data); err != nil {
		errs = append(errs, err.Error())
	}

	if err := r.SetFormat(f); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) != 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	return r, nil
}

type responderImpl struct {
	data   interface{}
	format OutputFormat
	status int
	pages  *ResponsePages
}

func (r *responderImpl) Validate() error       { return nil }
func (r *responderImpl) Data() interface{}     { return r.data }
func (r *responderImpl) Format() OutputFormat  { return r.format }
func (r *responderImpl) Status() int           { return r.status }
func (r *responderImpl) Pages() *ResponsePages { return r.pages }

func (r *responderImpl) AddData(d interface{}) error {
	if d == nil {
		return errors.New("cannot add nil data to responder")
	}

	if r.data != nil {
		return errors.New("cannot add new data to responder")
	}

	r.data = d
	return nil
}

func (r *responderImpl) SetFormat(o OutputFormat) error {
	if !o.IsValid() {
		return errors.New("invalid output format")
	}

	r.format = o
	return nil
}

func (r *responderImpl) SetStatus(s int) error {
	if http.StatusText(s) == "" {
		return fmt.Errorf("%d is not a valid HTTP status", s)
	}

	r.status = s
	return nil
}

func (r *responderImpl) SetPages(p *ResponsePages) error {
	if err := p.Validate(); err != nil {
		return errors.Wrap(err, "cannot set an invalid page definition")
	}

	r.pages = p
	return nil
}
