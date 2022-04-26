/*
Package model maps database models to API models for REST requests to the server.

Adding Models

Each model is kept in the model package of the REST v2 API in its own file.
To create a new model, define a struct containing all of the fields that it will return
and implement its two main interface methods BuildFromService and ToService.
Be sure to include struct tags to define the names the fields will have when
serialized to JSON.

Guidelines for Creating Models

Include as much data as a user is likely to want when inspecting this resource.
This is likely to be more information than seems directly needed, but there is
little penalty to its inclusion.

Return an error when type casting fails.

Model Methods

The Model type is an interface with two methods.

				BuildFromService(in interface{}) error

BuildFromService fetches all needed data from the passed in object and sets them
on the model. BuildFromService may sometimes be called multiple times with different
types that all contain data to build up the model object. In this case, a type switch
is likely necessary to determine what has been passed in.

				ToService()(interface{}, error)

ToService creates an as-complete-as-possible version of the service layer's version
of this model. For example, if this is is a REST v2 Task model, the ToService method
creates a service layer Task and sets all of the fields it is able to and returns it.
*/
package model
