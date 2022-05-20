package model

// Model defines how an API resource which will be both taken from requests and
// turned into service layer models and taken from service layer models and
// turned into API models to be returned.
// Unless there's a specific need to implement it, in general it is neither
// necessary nor recommended to implement this model due to its reliance on
// interface{}; instead, pass in and return the actual service model that
// corresponds to the REST model.
type Model interface {
	BuildFromService(interface{}) error
	ToService() (interface{}, error)
}
