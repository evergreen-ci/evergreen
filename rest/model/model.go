package model

// Model defines how an API resource which will be both taken from requests and
// turned into service layer models and taken from service layer models and
// turned into api models to be returned.
type Model interface {
	BuildFromService(interface{}) error
	ToService() (interface{}, error)
}
