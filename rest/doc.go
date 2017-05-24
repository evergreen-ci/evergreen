/*
	The REST API V2 has a series of central types that are useful to understand when
	adding new endpoints or increasing its functionality.

	Model

	Models are structs that represent the object returned by the API. A model
	is an interface with two methods, BuildFromService and ToService that
	define how to transform to and from an API model.

	Connector

	Connector is a very large interface that defines interaction with the backing
	database and service layer. It has two main sets of implementations: one that
	communicate with the database and one that mocks the same functionality. Development
	does not require the creation of new Connector's, only addition to extant ones.

	RequestHandler

	RequestHandler is an interface which defines how to process an HTTP request
	for an API resource. RequestHandlers implement methods for fetching a new copy
	of the RequestHandler, ParseAndValidate the request body and how to execute
	the bulk of the request against the database.

	MethodHandler

	MethodHandler is an struct that contains all of the data necessary for
	completing an API request. It contains an Authenticator to handle access control.
	Authenticator is an interface type with many implementations that permit user's
	access based on different sets of criteria. MethodHandlers also contain functions
	for attaching data to a request and a RequestHandler to execute the request.

	RouteManager

	RouteManagers are structs that define all of the functionality of a particular
	API route, including definitions of each of the methods it implements and the
	path used to access it.

	PaginationExecutor

	PaginationExecutor is a type that handles gathering necessary information for
	paginating and handles executing the necessary parts of the API request. The
	PaginationExecutor type is an implemention of the RequestHandler so that writing paginated
	endpoints does not necessarily require rewriting these methods; However, any of
	these methods may be overwritten to provide additional flexibility and funcitonality.

	PaginatorFunc

	PaginatorFunc is a function type that defines how to perform pagination over a
	specific resource. It is the only type that is required to be implemented when
	adding paginated API resources.

*/
package rest
