mciModule
  .constant('API_V1', {
    BASE: '/rest/v1',

    // ## PROJECTS API ##
    PROJECTS_API: _.template('{base}/projects/'),
    PROJECTS_DETAIL_API: _.template('{base}/projects/{project_id}'),
    PROJECTS_VERSION_API: _.template('{base}/projects/{project_id}/versions'),
    PROJECTS_REVISIONS_API: _.template('{base}/projects/{project_id}/revisions/{revision}'),
    PROJECTS_HISTORY_API: _.template('{base}/projects/{project_id}/test_history'),
    PROJECTS_LAST_GREEN_API: _.template('{base}/projects/{project_id}/last_green'),

    // TODO ## PATCHES API ##

    // ## VERSIONS API ##
    VERSIONS_BY_ID: _.template('{base}/versions/{version_id}'),

    // ## BUILDS API ##
    BUILDS_DETAIL: _.template('{base}/builds/{build_id}'),
    BUILDS_STATUS: _.template('{base}/builds/{build_id}/status'),

    // TODO ## TASKS API ##

    // TODO ## SCHEDULER API ##
    // WATERFALL API
    WATERFALL_VERSIONS_ROWS: _.template('{base}/waterfall/{project_id}/versions'),
  })

  .constant('API_TASKDATA', {
    BASE: '/plugin/json',
    TASK_BY_ID: _.template('{base}/task/{task_id}/{name}/'),
    TASK_BY_TAG: _.template('{base}/tag/{project_id}/{tag}/{variant}/{task_name}/{name}'),
    TASK_BY_COMMIT: _.template('{base}/commit/{project_id}/{revision}/{variant}/{task_name}/{name}'),
    TASK_HISTORY: _.template('{base}/history/{task_id}/{name}'),

    PROJECT_TAGS: _.template('{base}/tags/'),
  })

  // Multi Page App User Interface routes
  .constant('MPA_UI', {
    BUILD_BY_ID: _.template('/build/{build_id}'),
    TASK_BY_ID: _.template('/task/{task_id}'),
  })
