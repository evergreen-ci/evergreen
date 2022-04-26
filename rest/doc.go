/*
Package rest provides an API for Evergreen users, as well as the GraphQL interface.

The REST API V2 has a series of central types that are useful to understand when
adding new endpoints or increasing its functionality.

Model

Models are structs that represent the object returned by the API. A model
is an interface with two methods, BuildFromService and ToService that
define how to transform to and from an API model.

Connector

Connector is a very large interface that defines interaction with the backing
database and service layer. It has two main implementations: one that
communicates with the database and one that mocks the same functionality. Development
does not require the creation of new Connectors, only addition to extant ones.

RequestHandler

RequestHandler is an interface which defines how to process an HTTP request
for an API resource. RequestHandlers implement methods for fetching a new copy
of the RequestHandler, ParseAndValidate the request body and how to execute
the bulk of the request against the database.

MethodHandler

MethodHandler is a struct that contains all of the data necessary for
completing an API request. It contains an Authenticator to handle access control.
Authenticator is an interface type with many implementations that permit users'
access based on different sets of criteria. MethodHandlers also contain functions
for attaching data to a request and a RequestHandler to execute the request.

RouteManager

RouteManagers are structs that define all of the functionality of a particular
API route, including definitions of each of the methods it implements and the
path used to access it.
*/
package rest
