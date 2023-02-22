/*
Package data provides database access for API requests to the server.

# Adding to the Connector

The Connector is an interface to define how to access methods that access the
database and also have special mocking needs for testing (e.g. access to an
external service). Methods that have no needs other than access to the database
do not need to be put in the Connector interface.

To add to the Connector, add the method signature into the Connector interface.
rest/data/interface.go. Next, add the implementation that interacts with the
database to the database backed Connector implementation. Finally, add a mock
implementation to the mock Connector implementation.

Implementing database backed methods requires using methods in Evergreen's model
package. As much database specific information as possible should be kept out of
these methods. For example, if a new aggregation pipeline is needed to complete
the request, it should be defined in the Evergreen model package or relevant
subpackage.
*/
package data
