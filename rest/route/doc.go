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
	mux Context for that request and can be fetched using the context.Get function
	and providing it with the correct key for the desired data.

	From the Route Variables

	Variables from routes defined with variables such as /tasks/{task_id} can be
	fetched using calls to the mux.Vars function and providing the variable name
	to the returned map. For example, the taskId of that route could be fetched using:

					mux.Vars(r)["task_id"]

	From the URL Query Parameters

	To fetch variables from the URL query parameters, get it from the http.Request's
	URL object using:

					r.URL.Query().Get("status")

	Finally, the Execute method is the only method with access to the Connector
	and is therefore capable of making calls to the backing database to fetch and alter
	its state. The Execute method should use the parameters gathered in the ParseAndValidate
	method to implement the main logic and functionality of the request.

	Pagination

	PaginationExecutor is a struct that already implements the RequestHandler interface.
	To create a method with pagination, the only function that is needed is a PaginatorFunc.

	PaginatorFunc

	A PaginatorFunc defines how to paginate over a resource given a key to start pagination
	from and a limit to limit the number of results. PaginatorFunc has the following signature:

					func(key string, limit int, args interface{}, sc Connector)([]Model, *PageResult, error)

	The key and limit are fetched automatically by the PaginationExecutor's ParseAndValidate
	function. These parameters should be used to query for the correct set of results.

	The args is a parameter that may optionally be used when more information is
	needed to completed the request. To populate this field, the RequestHandler that
	wraps the PaginationExecutor must implement a ParseAndValidate method that overwrites
	the PaginationExecutor's and then calls it with the resulting request for example,
	a RequestHandler called fooRequestHandler that needs additional args would look
	like:

					fooRequestHandler{
					 *PaginationExecutor
					}

					extraFooArgs{
						extraParam string
					}

					func(f *fooRequesetHandler) ParseAndValidate(r *http.RequestHandler) error{
						urlParam := r.URL.Query().Get("extra_url_param")
						f.PaginationExecutor.Args = extraFooArgs{urlParam}

						return f.PaginationExecutor.ParseAndValidate(r)
					}

					func fooRequestPaginator(key string, limit int, args interface{},
						 sc data.Connector)([]model.Model, *PageResult, error){

						 fooArgs, ok := args.(extraFooArgs)
						 if !ok {
							// Error
						 }

					...
					}


	PageResult

	The PageResult is a struct that must be constructed and returned by a PaginatorFunc
	It contains the information used for creating links to the next and previous page of
	results.

	To construct a Page, you must provide it with the limit of the number of results
	for the page, which is either the default limit if none was provided, the limit
	of the previous request if provided, or the number of documents between the page
	and the end of the result set. The end of the result set is either the beginning of
	set of results currently being returned if constructing a previous page, or the end
	of all results if constructing a next page.

	The Page must also contain the key of the item that begins the Page of results.
	For example, when creating a next Page when fetching a page of 100 tasks, the
	task_id of the 101st task should be used as the key for the next Page.

	If the page being returned is the first or last page of pagination, then there
	is no need to create that Page.

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
