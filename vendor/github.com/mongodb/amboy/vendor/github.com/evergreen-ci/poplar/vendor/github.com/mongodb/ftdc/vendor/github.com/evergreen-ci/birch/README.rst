==============================================
``birch`` -- Go Lang BSON Manipulation Package
==============================================

Overview
--------

The ``birch`` package provides an API for manipulating bson in Go programs
without needing to handle byte slices or maintain types for marshalling bson
into using reflection.

This code is an evolution from an earlier phase of development of the official
`MongoDB Go Driver's BSON library
<https://godoc.org/go.mongodb.org/mongo-driver/bson>`_, but both libraries have
diverged. For most application uses the official BSON library is a better choice
for interacting with BSON. 

The Document type in this library implements bson library's Marhsaler and
Unmarshaller interfaces, to support improved interoperation.

API and Documentation
---------------------

See the `godoc API documentation
<http://godoc.org/github.com/evergreen-ci/birch>` for more information
about amboy interfaces and internals.


Development
-----------

Please file all issues in the `MAKE project
<https://jira.mongodb.org/browse/MAKE>`_ in the `MongoDB Jira
<https://jira.mongodb.org/>`_ instance.

Future Work
-----------

- Benchmarks for lookups and iteration.

- Complete test coverage.

- Improved interfaces to more consistently avoid panics.

- Remove use of byte-constants for bsontype names throughout the package.
