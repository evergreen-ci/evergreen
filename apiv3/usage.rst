=====
Usage
===== 

General Functionality
=====================


Content Type and Communication
------------------------------
 The API accepts and returns all results in JSON. Some resources also allow 
URL parameters to provide additional specificity to a request.

Errors
------
 When an error is encountered during a request, the API returns a JSON 
object with the HTTP status code and a message describing the error of the form:

::

  {
   "status": <http_status_code>,
   "error": <error message>
  }

Pagination
----------
 API Routes that fetch many objects return them in a JSON array and support paging 
through subsets of the total result set. When there are additional results for the 
query, access to them is populated in a 'Links' HTTP header. This header has the form: 

::

 "Links" : <http://<EVERGREEN_HOST>/rest/v2/path/to/resource?start_at=<pagination_key>&limit=<objects_per_page>; rel="next"
<http://<EVERGREEN_HOST>/rest/v2/path/to/resource?start_at=<pagination_key>&limit=<objects_per_page>; rel="prev"

Dates
-----

 Date fields are returned and accepted in ISO-8601 UTC extended format. They contain 
3 fractional seconds with a ‘dot’ separator. 

Empty Fields
------------

 A returned object will always contain its complete list of fields. Any field 
that does not have an associated value will be filled with JSON's null value. 

Resources
=========

The API has a series of implemented objects that it returns depending on the 
queried endpoint. 

Task
----

``Base URL``: http://evergreen.mongodb.com/rest/v2/

 The task is a basic unit of work understood by evergreen. They usually comprise 
a suite of tests or generation of a set of artifacts. 

Objects
~~~~~~~

.. list-table:: **Task**
   :widths: 25 10 55
   :header-rows: 1

   * - Name
     - Type
     - Description
   * - ``task_id``               
     - string         
     - Unique identifier of this task
   * - ``create_time``           
     - time           
     - Time that this task was first created
   * - ``dispatch_time``         
     - time           
     - Time that this time was dispatched
   * - ``push_time``             
     - time           
     - Time that the git commit associated with this task was pushed to github
   * - ``scheduled_time``        
     - time           
     - Time that this task is scheduled to begin
   * - ``start_time``            
     - time           
     - Time that this task began execution
   * - ``finish_time``           
     - time           
     - Time that this task finished execution
   * - ``version_id``            
     - string         
     - An identifier of this task by its project and commit hash
   * - ``branch``                
     - string         
     - The version control branch that this task is associated with
   * - ``revision``              
     - string         
     - The version control identifier associated with this task
   * - ``priority``              
     - int            
     - The priority of this task to be run
   * - ``activated``             
     - boolean        
     - Whether the task is currently active
   * - ``activated_by``          
     - string         
     - Identifier of the process or user that activated this task
   * - ``build_id``              
     - string         
     - Identifier of the build that this task is part of
   * - ``distro_id``             
     - string         
     - Identifier of the distro that this task runs on
   * - ``build_variant``         
     - string         
     - Name of the buildvariant that this task runs on
   * - ``depends_on``            
     - array          
     - List of task_ids of task that this task depends on before beginning
   * - ``display_name``          
     - string         
     - Name of this task displayed in the UI
   * - ``host_id``               
     - string         
     - The ID of the host this task ran or is running on
   * - ``restarts``              
     - int            
     - Number of times this task has been restarted
   * - ``execution``             
     - int            
     - The number of the execution of this particular task
   * - ``order``                 
     - int            
     - The position in the commit history of commit this task is associated with
   * - ``status``                
     - string         
     - The current status of this task
   * - ``status_details``        
     - status_object  
     - Object containing additional information about the status
   * - logs                  
     - logs_object    
     - Object containing additional information about the logs for this task
   * - time_taken_ms         
     - int            
     - Number of milliseconds this task took during execution
   * - expected_duration_ms  
     - int            
     - Number of milliseconds expected for this task to execute


.. list-table:: **Logs**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - agent_log   
     - string  
     - Link to logs created by the agent process
   * - task_log       
     - string  
     - Link to logs created by the task execution
   * - system_log  
     - string  
     - Link to logs created by the machine running the task
   * - all_log     
     - string  
     - Link to logs containing merged copy of all other logs

.. list-table:: **Status**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - status     
     - string   
     - The status of the completed task
   * - type       
     - string   
     - The method by which the task failed
   * - desc       
     - string   
     - Description of the final status of this task
   * - timed_out  
     - boolean  
     - Whether this task ended in a timeout

Endpoints
~~~~~~~~~

List Tasks By Build
```````````````````

::

 GET /builds/<build_id>/tasks

 List all tasks within a specific build


.. list-table:: **Parameters**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - start_at     
     - string   
     - Optional. The identifier of the task to start at in the pagination
   * - limit       
     - int   
     - Optional. The number of tasks to be returned per page of pagination. Defaults to 100

List Tasks By Project And Commit
````````````````````````````````

::

 GET /project/<project_name>/versions/<commit_hash>/tasks

 List all tasks within a commit of a given project

.. list-table:: **Parameters**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - start_at     
     - string   
     - Optional. The identifier of the task to start at in the pagination
   * - limit       
     - int   
     - Optional. The number of tasks to be returned per page of pagination. Defaults to 100


Get A Single Task
`````````````````

::

 GET /tasks/<task_id>

 Fetch a single task using its ID

Restart A Task
``````````````

::

 POST /tasks/<task_id>/restart

 Restarts the task of the given ID. Can only be performed if the task is in progress.

Change A Task's Execution Status
````````````````````````````````

::

  PATCH /tasks/<task_id> 

  Change the current execution status of a task. Accepts a JSON body with the new 
task status to be set. 

.. list-table:: **Accepted Parameters**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - activated    
     - boolean   
     - The activation status of the task to be set to
   * - priority
     - int   
     - The priority of this task's execution. Limited to 100 for non-superusers


 For example, to set activate the task and set its status priority to 100, add 
 the following JSON to the request body:


 ::

 {
   "activated": true,
   "priority": 100
 }


Test
----

``Base URL``: http://evergreen.mongodb.com/rest/v2/

 A test is a sub-operation of a task performed by Evergreen. 

Objects
~~~~~~~

.. list-table:: **Test**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - task_id     
     - string  
     - Identifier of the task this test is a part of 
   * - Status   
     - string  
     - Execution status of the test
   * - test_file       
     - string  
     - Name of the test file that this test was run in
   * - logs  
     - test_log  
     - Object containing information about the logs for this test
   * - exit_code  
     - int
     - The exit code of the process that ran this test 
   * - start_time  
     - time
     - Time that this test began execution
   * - end_time  
     - time
     - Time that this test stopped execution

.. list-table:: **Test Logs**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - url     
     - string  
     - URL where the log can be fetched
   * - line_num   
     - int  
     - Line number in the log file corresponding to information about this test
   * - url_raw       
     - string  
     - URL of the unprocessed version of the logs file for this test
   * - log_id  
     - string  
     - Identifier of the logs corresponding to this test

Endpoints
~~~~~~~~~

Get Tests From A Task
`````````````````````

:: 

 GET /tasks/<task_id>/tests

 Fetches a paginated list of tests that ran as part of the given task 

.. list-table:: **Parameters**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - start_at     
     - string   
     - Optional. The identifier of the test to start at in the pagination
   * - limit       
     - int   
     - Optional. The number of tets to be returned per page of pagination. Defaults to 100
   * - status       
     - string   
     - Optional. A status of test to limit the results to.

Host
----

``Base URL``: http://evergreen.mongodb.com/rest/v2/

 The hosts resource defines the a running machine instance in Evergreen.

Objects
~~~~~~~

.. list-table:: **Host**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - host_id     
     - string  
     - Unique identifier of a specific host
   * - distro   
     - distro_info  
     - Object containing information about the distro type of this host
   * - started_by       
     - string  
     - Name of the process or user that started this host
   * - host_type  
     - string  
     - The instance type requested for the provider, primarily used for ec2 dynamic hosts
   * - user  
     - string
     - The user associated with this host. Set if this host was spawned for a specific user
   * - status  
     - string
     - The current state of the host 
   * - running_task
     - task_info
     - Object containing information about the task the host is currently running

.. list-table:: **Distro Info**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - distro_id     
     - string  
     - Unique Identifier of this distro. Can be used to fetch more informaiton about this distro
   * - provider   
     - string  
     - The service which provides this type of machine

.. list-table:: **Task Info**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - task_id     
     - string  
     - Unique Identifier of this task. Can be used to fetch more informaiton about this task
   * - name   
     - string  
     - The name of this task
   * - dispatch_time   
     - time  
     - Time that this task was dispatched to this host
   * - version_id
     - string  
     - Unique identifier for the version of the project that this task is run as part of
   * - build_id
     - string  
     - Unique identifier for the build of the project that this task is run as part of


Endpoints
~~~~~~~~~

Fetch All Hosts
```````````````

::

 GET /hosts

 Returns a paginated list of all hosts in Evergreen

.. list-table:: **Parameters**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - start_at     
     - string   
     - Optional. The identifier of the host to start at in the pagination
   * - limit       
     - int   
     - Optional. The number of hosts to be returned per page of pagination. Defaults to 100
   * - status       
     - string   
     - Optional. A status of host to limit the results to
