/*
	Adding to the Connector

	The Connector is a very large interface that defines how to access the main
	state of the database and data central to Evergreen's function. All methods of the
	Connector are contained in the data package in files depending
	on the main type they allow access to (i.e. all test access is contained in data/test.go).

	Extending Connector should only be done when the desired functionality cannot be performed
	using a combination of the methods it already contains or when such combination would
	be unseemingly slow or expensive.

	To add to the Connector, add the method signature into the interface in
	data/data.go. Next, add the implementation that interacts
	with the database to the database backed object. These objects are named by the
	resource they allow access to. The object that allows access to Hosts is called
	DBHostConnector. Finally, add a mock implementation to the mock object. For
	Hosts again, this object would be called MockHostConnector.

	Implementing database backed methods requires using methods in Evergreen's model
	package. As much database specific information as possible should be kept out of
	the these methods. For example, if a new aggregation pipeline is needed to complete
	the request, it should be defined in the Evergreen model package and used only
	to aggregate in the method.
*/
package data
