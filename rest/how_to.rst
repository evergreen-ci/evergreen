======================
REST V2 Use Case Guide
======================

 This guide presents users with information on how to perform some commonly 
 desired actions in the Evergreen API.

Find all failures of a given build
----------------------------------

Endpoint
~~~~~~~~~

``GET /builds/<build_id>/tasks`` 

Description
~~~~~~~~~~~

To better understand the state of a build, perhaps when attempting to determine 
the quality of a build for a release, it is helpful to be able to fetch information 
about the tasks it ran. To fetch this data, make a call to the ``GET /builds/<build_id>/tasks`` 
endpoint. Page through the results task data to produce meaningful statistics 
like the number of task failures or percentage of failures of a given build.

Find detailed information about the status of a particular tasks and its tests
------------------------------------------------------------------------------

Endpoints
~~~~~~~~~

``GET /tasks/<task_id>`` 

``GET /tasks/<task_id>/tests``

Description
~~~~~~~~~~~

To better understand all aspects of a task failure, such as failure mode, 
which tests failed, and how long it took, it is helpful to fetch this information
about a task. This can be accomplished using 2 API calls. Make one call  
to the endpoint for a specific task ``GET /tasks/<task_id>`` which returns
information about the task itself. Then, make a second cal to ``GET /tasks/<task_id>/tests``
which delivers specific information about the tests that ran in a certain task.

Get all hosts
-------------

Endpoint
~~~~~~~~~

``GET /hosts``

Description
~~~~~~~~~~~

Retrieving information on Evergreen's hosts can be helpful for system 
monitoring. To fetch this information, make a call to ``GET /hosts``, which 
returns a paginated list of hosts. Page through the results to inspect all hosts.

Restart all failures for a commit
---------------------------------

Endpoints
~~~~~~~~~

``GET /project/<project_name>/versions/<commit_hash>/tasks`` 

``POST /tasks/<task_id>/restart`` 

Description
~~~~~~~~~~~

Some Evergreen projects contain flaky tests or can endure spurious failures.
To restart all of these tasks to gain better signal a user can fetch all of 
the tasks for a commit. Make a request to to ``GET /project/<project_name>/versions/<commit_hash>/tasks`` 
to fetch the tasks that ran and then loop over all of the returned tasks, 
calling ``POST /tasks/<task_id>/restart`` on each task which has failed.
