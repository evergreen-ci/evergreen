mciModule
  .constant('API_V1', {
    BASE: '/rest/v1',
    VERSION_API: _.template('/version/{buildId}/'),
    BUILD_API: '/rest/v1/build/',

    // ## PROJECTS API ##
    PROJECTS_API: _.template('{base}/projects/'),
    PROJECTS_DETAIL_API: _.template('{base}/projects/{project_id}'),
    PROJECTS_VERSION_API: _.template('{base}/projects/{project_id}/versions'),
    PROJECTS_REVISIONS_API: _.template('{base}/projects/{project_id}/revisions/{revision}'),
    PROJECTS_HISTORY_API: _.template('{base}/projects/{project_id}/test_history'),
    PROJECTS_LAST_GREEN_API: _.template('{base}/projects/{project_id}/last_green'),

    // TODO ## PATCHES API ##

    // TODO ## VERSIONS API ##

    // ## BUILDS API ##
    BUILDS_DETAIL: _.template('{base}/builds/{build_id}'),
    BUILDS_STATUS: _.template('{base}/builds/{build_id}/status'),

    // TODO ## TASKS API ##

    // TODO ## SCHEDULER API ##
  })
  .constant('API_TASKDATA', {
    BASE: '/plugin/json',
    TASK_BY_ID: _.template('{base}/task/{task_id}/{name}/'),
    TASK_COMMIT: _.template('{base}/commit/{project_id}/{revision}/{variant}/{task_name}/{name}'),
    TASK_HISTORY: _.template('{base}/history/{task_id}/{name}'),
  })
