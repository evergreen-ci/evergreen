/*
	Adding a Route

	Adding a new route to the REST v2 API requires creation of a few structs and
	implementation of a few new methods.

	RequestHandler

	The RequestHandler is a central interface in the REST v2 API with following
	signature:

					type RequestHandler interface {
						Handler() RequestHandler
						ParseAndValidate(*http.Request) error
						Execute(data.Connector) (ResponseData, error)
					}

	RequestHandlers should be placed in files in the route/ directory depending on the type
	of resource they access.

	To add a new route you must create a struct that implements its three main interface
	methods. The Handler method must return a new copy of the RequestHandler so that
	a new copy of this object can be used on successive calls to the same endpoint.

	The ParseAndValidate method is the only method that has access to the http.Request object.
	All necessary query parameters and request body information must be fetched from the request
	in this function and added to the struct for use in the Execute function. These fetches can
	take a few main forms:

	From mux Context

	Data gathered before the main request by the PrefetchFunc's are attached to the
	mux Context for that request and can be fetched from request's context
	and providing it with the correct key for the desired data.

	From the Route Variables

	Variables from routes defined with variables such as /tasks/{task_id} can be
	fetched using calls to the gimlet.GetVars function and providing the variable name
	to the returned map. For example, the taskId of that route could be fetched using:

					gimlet.GetVars(r)["task_id"]

	From the URL Query Parameters

	To fetch variables from the URL query parameters, get it from the http.Request's
	URL object using:

					r.URL.Query().Get("status")

	Finally, the Execute method is the only method with access to the Connector
	and is therefore capable of making calls to the backing database to fetch and alter
	its state. The Execute method should use the parameters gathered in the ParseAndValidate
	method to implement the main logic and functionality of the request.

	MethodHandler

	The MethodHandler type contains all data and types for complete execution of an
	API method. It holds:

	- A list of PrefetchFuncs used for grabbing data needed before
	the main execution of a method, such as user data for authentication

	- The HTTP method type (GET, PUT, etc.)

	- The Authenticator this method uses to control access to its data

	- The RequestHandler this method uses for the main execution of its request

	A MethodHandler is a composition of defined structs and functions that in total
	comprise the method. Many Authenticator and PrefetchFunc are already implemented
	and only need to be attached to this object to create the method once the Requesthandler
	is complete.

	RouteManager

	The RouteManager type holds all of the methods associated with a particular API
	route. It holds these as an array of MethodHandlers. It also contains the path
	by which this route may be accessed, and the version of the API that this is
	implemented as part of.

	When adding to the API there may already be a RouteManger in existence for the
	method being devloped. For example, if a method for GET /tasks/<task_id> is
	already implemented, then a new route manager is not required when createing POST /tasks/<task_id>.
	Its implementation only needs to be added to the existing RouteManager.

	Once created, the RouteManager must be registered onto the mux.Router with the
	Connector by calling

	     route.Register(router, serviceContext).
*/
package route
