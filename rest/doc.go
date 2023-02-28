/*
Package rest provides an API for Evergreen users, as well as the GraphQL interface.

The REST API V2 has a series of central types that are useful to understand when
adding new endpoints or increasing its functionality.

# Model

Models are structs that represent the object returned by the API. A model
is an interface with two methods, BuildFromService and ToService that
define how to transform to and from an API model. This interface only needs to
be satisfied for specific API models; it is not necessary for every single API
model to implement it unless there is a reason to do so (e.g. reflection).

# Connector

Connector is an interface that defines interaction with the backing database and
service layer for operations that rely on mocks for testing purposes. It has
two main implementations: one that communicates with the database (and
potentially other services) and one that mocks the same functionality.

# RequestHandler

RequestHandler is an interface which defines how to process an HTTP request
for an API resource. RequestHandlers implement methods for fetching a new copy
of the RequestHandler, ParseAndValidate the request body and how to execute
the bulk of the request against the database.

# MethodHandler

MethodHandler is a struct that contains all of the data necessary for
completing an API request. It contains an Authenticator to handle access control.
Authenticator is an interface type with many implementations that permit users'
access based on different sets of criteria. MethodHandlers also contain functions
for attaching data to a request and a RequestHandler to execute the request.

# RouteManager

RouteManagers are structs that define all of the functionality of a particular
API route, including definitions of each of the methods it implements and the
path used to access it.
*/
package rest
