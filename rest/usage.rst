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
   * - ``logs``
     - logs_object    
     - Object containing additional information about the logs for this task
   * - ``time_taken_ms``
     - int            
     - Number of milliseconds this task took during execution
   * - ``expected_duration_ms``
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

Abort A Task
````````````

::

 POST /tasks/<task_id>/abort

 Abort the task of the given ID. Can only be performed if the task is in progress.

Change A Task's Execution Status
````````````````````````````````

::

  PATCH /tasks/<task_id> 

  Change the current execution status of a task. Accepts a JSON body with the new task status to be set.

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

Fetch Host By ID
````````````````

::

 GET /hosts/<host_id>

 Fetches a single host using its ID   

Patch
-----

 A patch is a manually initiated version submitted to test local changes.

Objects
~~~~~~~

.. list-table:: **Patch**
   :widths: 25 10 55
   :header-rows: 1

   * - Name
     - Type
     - Description
   * - patch_id
     - string
     - Unique identifier of a specific patch
   * - description
     - string
     - Description of the patch
   * - project_id
     - string
     - Name of the project
   * - branch
     - string
     - The branch on which the patch was initiated
   * - git_hash
     - string
     - Hash of commit off which the patch was initiated
   * - patch_number
     - int
     - Incrementing counter of user's patches
   * - author
     - string
     - Author of the patch
   * - status
     - string
     - Status of patch
   * - create_time
     - time
     - Time patch was created
   * - start_time
     - time
     - Time patch started to run
   * - finish_time
     - time
     - Time at patch completion
   * - build_variants
     - string[]
     - List of identifiers of builds to run for this patch
   * - tasks
     - string[]
    - List of identifiers of tasks used in this patch
   * - variants_tasks
     - variant_task[]
     - List of documents of available tasks and associated build variant
   * - activated
     - bool
     - Whether the patch has been finalized and activated

.. list-table:: **Variant Task**
   :widths: 25 10 55
   :header-rows: 1

   * - Name
     - Type
     - Description
   * - name
     - string
     - Name of build variant
   * - tasks
     - string[]
     - All tasks available to run on this build variant

Endpoints
~~~~~~~~~

Fetch Patches By Project
````````````````````````

::

 GET /projects/<project_id>/patches

 Returns a paginated list of all patches associated with a specific project

.. list-table:: **Parameters**
   :widths: 25 10 55
   :header-rows: 1

   * - Name
     - Type
     - Description
   * - start_at
     - string
     - Optional. The create_time of the patch to start at in the pagination. Defaults to now
   * - limit
     - int
     - Optional. The number of patches to be returned per page of pagination. Defaults to 100

Fetch Patches By User
`````````````````````

::

 GET /users/<user_id>/patches

 Returns a paginated list of all patches associated with a specific user

.. list-table:: **Parameters**
  :widths: 25 10 55
  :header-rows: 1

   * - Name
     - Type
     - Description
   * - start_at
     - string
     - Optional. The create_time of the patch to start at in the pagination. Defaults to now
   * - limit
     - int
     - Optional. The number of patches to be returned per page of pagination. Defaults to 100

Fetch Patch By Id
`````````````````

::

 GET /projects/<project_id>/patches

 Fetch a single patch using its ID

Abort a Patch
`````````````

::

 POST /patches/<patch_id>/abort

 Aborts a single patch using its ID and returns the patch

Restart a Patch
```````````````

::

 POST /patches/<patch_id>/restart

 Restarts a single patch using its ID then returns the patch

Change Patch Status
```````````````````

::

 PATCH /patches/<patch_id>

 Sets the priority and activation status of a single patch to the input values

.. list-table:: **Parameters**
   :widths: 25 10 55
   :header-rows: 1

   * - Name
     - Type
     - Description
   * - priority
     - int
     - Optional. The priority to set the patch to
   * - status
     - bool
     - Optional. The activation status to set the patch to

Build
-----

 The build resource represents the combination of a version and a buildvariant.

Objects
~~~~~~~

.. list-table:: **Build**
   :widths: 25 10 55
   :header-rows: 1

   * - Name        
     - Type           
     - Description
   * - project_id     
     - string  
     - The identifier of the project this build represents
   * - create_time   
     - time  
     - Time at which build was created
   * - start_time       
     - time  
     - Time at which build started running tasks
   * - finish_time  
     - time  
     - Time at which build finished running all tasks
   * - push_time
     - time
     - If build was triggered by git commit, when the commit was pushed
   * - version  
     - string
     - The version this build is running tasks for
   * - branch
     - string
     - The branch of project the build is running
   * - gitspec
     - string
     - Hash of the revision on which this build is running
   * - build_variant
     - string
     - Build distro and architecture information
   * - status
     - string
     - The status of the build
   * - activated
     - bool
     - Whether this build was manually initiated
   * - activated_by
     - string
     - Who initiated the build
   * - activated_time
     - time
     - When the build was initiated
   * - order
     - int
     - Incrementing counter of project's builds
   * - tasks
     - string[]
     - The tasks to be run on this build
   * - time_taken_ms
     - int
     - How long the build took to complete all tasks
   * - display_name
     - string
     - Displayed title of the build showing version and variant running
   * - predicted_makespan_ms
     - int
     - Predicted makespan by the scheduler prior to execution
   * - actual_makespan_ms
     - int
     - Actual makespan measured during execution
   * - origin
     - string
     - The source of the patch, a commit or a patch

Endpoints
~~~~~~~~~

Fetch Build By Id
`````````````````

::

 GET /builds/<build_id>

 Fetches a single build using its ID

Abort a Build
`````````````

::

 POST /builds/<build_id>/abort

 Aborts a single build using its ID then returns the build

Restart a Build
```````````````

::

 POST /builds/<build_id>/restart

 Restarts a single build using its ID then returns the build

Change Build Status
```````````````````

::

 PATCH /builds/<build_id>

 Sets the priority and activation status of a single build to the input values

.. list-table:: **Parameters**
  :widths: 25 10 55
  :header-rows: 1

   * - Name
     - Type
     - Description
   * - priority
     - int
     - Optional. The priority to set the build to
   * - status
     - bool
     - Optional. The activation status to set the build to

Version
-------

 A version is a commit in a project.

Objects
~~~~~~~

.. list-table:: **Version**
  :widths: 25 10 55
  :header-rows: 1

  * - Name
    - Type
    - Description
  * - ``create_time``
    - time
    - Time that the version was first created
  * - ``start_time``
    - time
    - Time at which tasks associated with this version started running
  * - ``finish_time``
    - time
    - Time at which tasks associated with this version finished running
  * - ``revision``
    - string
    - The version control identifier
  * - ``author``
    - string
    - Author of the version
  * - ``author_email``
    - string
    - Email of the author of the version
  * - ``message``
    - string
    - Message left with the commit
  * - ``status``
    - string
    - The status of the version
  * - ``repo``
    - string
    - The github repository where the commit was made
  * - ``branch``
    - string
    - The version control branch where the commit was made
  * - ``build_variants_status``
    - []buildDetail
    - List of documents of the associated build variant and the build id

Endpoints
~~~~~~~~~

Fetch Version By Id
```````````````````

::

 GET /versions/<version_id>

 Fetches a single version using its ID

Abort a Version
```````````````

::

 POST /versions/<version_id>/abort

 Aborts a single version using its ID then returns the version

Restart a Version
`````````````````

::

 POST /versions/<version_id>/restart

 Restarts a single version using its ID then returns the version

Get Builds From A Version
`````````````````````````

::

 GET /versions/<version_id>/builds

 Fetches a list of builds associated with a version

Project
-------

 A project corresponds to a single branch of a repository.

Objects
~~~~~~~

.. list-table:: **Project**
  :widths: 25 10 55
  :header-rows: 1

   * - Name
     - Type
     - Description
   * - batch_time
     - int
     - Unique identifier of a specific patch
   * - branch_name
     - string
     - Name of branch
   * - display_name
     - string
     - Project name displayed to users
   * - enabled
     - bool
     - Whether evergreen is enabled for this project
   * - identifier
     - string
     - Internal evergreen identifier for project
   * - owner_name
     - string
     - Owner of project repository
   * - private
     - bool
     - A user must be logged in to view private projects
   * - remote_path
     - string
     - Path to config file in repo
   * - repo_name
     - string
     - Repository name
   * - tracked
     - bool
     - Whether the project is discoverable in the UI
   * - alert_settings
     - map[string][]alertConfig
     - Map of alert triggers to list of corresponding configs
   * - deactivate_previous
     - bool
     - List of identifiers of tasks used in this patch
   * - admins
     - []string
     - Usernames of project admins
   * - vars
     - map[string][string]
     - Map of project variables

.. list-table:: **Alert Config**
  :widths: 25 10 55
  :header-rows: 1

   * - Name
     - Type
     - Description
   * - provider
     - string
     - Name of alert provider
   * - settings
     - map[string]string
     - Settings defined for this alert config

Endpoints
~~~~~~~~~

Fetch all Projects
``````````````````

::

 GET /projects

 Returns a paginated list of all projects

.. list-table:: **Parameters**
  :widths: 25 10 55
  :header-rows: 1

   * - Name
     - Type
     - Description
   * - start_at
     - string
     - Optional. The id of the project to start at in the pagination. Defaults to empty string
   * - limit
     - int
     - Optional. The number of projects to be returned per page of pagination. Defaults to 100

DistroCost
----------

 A distro cost represents cost data and other relevant information regarding the distro for the cost reporting project.

Objects
~~~~~~~

.. list-table:: **DistroCost**
  :widths: 25 10 55
  :header-rows: 1

  * - Name
    - Type
    - Description
  * - ``distro_id``
    - string
    - The identifier of the distro
  * - ``sum_time_taken``
    - time.Duration
    - Aggregated duration of tasks belonging to the distro
  * - ``provider``
    - string
    - The cloud provider for the distro
  * - ``instance_type``
    - string
    - The type of the instance on which the distro runs 

Endpoints
~~~~~~~~~

Fetch Distro Cost By Distro ID
``````````````````````````````

::

 GET /cost/distro/<distro_id>

 Fetches distro cost associated with the specific distro filtered by time range

.. list-table:: **Parameters**
  :widths: 25 10 55
  :header-rows: 1

  * - Name
    - Type
    - Description
  * - ``starttime``
    - string
    - Lower bound of the time range
  * - ``duration``
    - string
    - Duration of the time range

TaskCost
----------

 A task cost represents cost data and other relevant information regarding the task for the cost reporting project.

Objects
~~~~~~~

.. list-table:: **TaskCost**
  :widths: 25 10 55
  :header-rows: 1

  * - Name
    - Type
    - Description
  * - ``task_id``
    - string
    - The identifier of the task
  * - ``display_name``
    - string
    - Name of this task displayed in the UI
  * - ``distro``             
    - string         
    - Identifier of the distro that this task runs on
  * - ``build_variant``         
    - string         
    - Name of the build variant that this task runs on
  * - ``time_taken``
    - time.Duration
    - Number of milliseconds this task took during execution
  * - ``githash``
    - string
    - The version control identifier associated with this task

Endpoints
~~~~~~~~~

Fetch Task Cost By ProjectID
``````````````````````````````

::

 GET /cost/project/<project_id>/tasks

 Returns a paginated list of all tasks associated with a specific project filtered by time range

.. list-table:: **Parameters**
  :widths: 25 10 55
  :header-rows: 1

  * - Name
    - Type
    - Description
  * - ``starttime``
    - string
    - Lower bound of the time range
  * - ``duration``
    - string
    - Duration of the time range
  * - ``start_at``    
    - string   
    - Optional. The identifier of the task to start at in the pagination
  * - ``limit``       
    - int   
    - Optional. The number of tasks to be returned per page of pagination. Defaults to 100

VersionCost
-----------

 A version cost represents cost data and other relevant information regarding the version for the cost reporting project.

Objects
~~~~~~~

.. list-table:: **VersionCost**
  :widths: 25 10 55
  :header-rows: 1

  * - Name
    - Type
    - Description
  * - ``version_id``
    - string
    - The identifier of the version
  * - ``sum_time_taken``
    - time.Duration
    - Aggregated duration of tasks belonging to the version

Endpoints
~~~~~~~~~

Fetch Version Cost By VersionID
```````````````````````````````

::

 GET /cost/version/<version_id>

 Fetches version cost associated with the specific version

Keys
----

Objects
~~~~~~~

.. list-table:: **Key**
   :widths: 25 10 55
   :header-rows: 1

   * - Name
     - Type
     - Description
   * - name
     - string
     - The unique name of the public key
   * - key
     - string
     - The public key, (e.g: 'ssh-rsa ...')

Endpoints
~~~~~~~~~

Fetch Current User's SSH Public Keys
````````````````````````````````````

::

 GET /keys

 Fetch the SSH public keys of the current user (as determined by the
 Api-User and Api-Key headers) as an array of Key objects.

 If the user has no public keys, expect: []
