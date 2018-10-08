mciModule
  // Misc Evergreen constants
  .constant('EVG', {
    GIT_HASH_LEN: 40,
    PATCH_ID_LEN: 24,
  })

  .constant('FORMAT', {
    ISO_DATE: 'YYYY-MM-DD',
  })

  .constant('BF', {
    OPEN_STATUSES: ['Open', 'In Progress', 'Waiting for bug fix'],
  })

  .constant('STITCH_CONFIG', {
    PERF: {
      serviceType: 'mongodb',
      serviceName: 'mongodb-atlas',
      appId: 'evergreen_perf_plugin-wwdoa',
      // Assets associated with this instance
      DB_PERF: 'perf',
      COLL_CHANGE_POINTS: 'change_points',
      COLL_UNPROCESSED_POINTS: 'unprocessed_change_points',
      COLL_PROCESSED_POINTS: 'processed_change_points',
      COLL_BUILD_FAILURES: 'build_failures',
    }
  })

  .constant('API_V1', {
    BASE: '/rest/v1',

    // ## PROJECTS API ##
    PROJECTS_API: 'projects/',
    PROJECTS_DETAIL_API: 'projects/{project_id}',
    PROJECTS_VERSION_API: 'projects/{project_id}/versions',
    PROJECTS_REVISIONS_API: 'projects/{project_id}/revisions/{revision}',
    PROJECTS_HISTORY_API: 'projects/{project_id}/test_history',
    PROJECTS_LAST_GREEN_API: 'projects/{project_id}/last_green',

    // TODO ## PATCHES API ##

    // ## VERSIONS API ##
    VERSIONS_BY_ID: 'versions/{version_id}',

    // ## BUILDS API ##
    BUILDS_DETAIL: 'builds/{build_id}',
    BUILDS_STATUS: 'builds/{build_id}/status',

    // TODO ## TASKS API ##

    // TODO ## SCHEDULER API ##
  })

  .constant('API_V2', {
    BASE: '/rest/v2',
    PATCH_BY_ID: 'patches/{patch_id}',
    VERSION_BY_ID: 'versions/{version_id}',
    RECENT_VERSIONS: 'projects/{project_id}/recent_versions',
    PROJECT_TASKS: 'projects/{project_id}/versions/tasks',
  })

  .constant('API_TASKDATA', {
    BASE: '/plugin/json',
    TASK_BY_ID: 'task/{task_id}/{name}/',
    TASK_BY_TAG: 'tag/{project_id}/{tag}/{variant}/{task_name}/{name}',
    TASK_BY_COMMIT: 'commit/{project_id}/{revision}/{variant}/{task_name}/{name}',
    TASK_HISTORY: 'history/{task_id}/{name}',

    PROJECT_TAGS: 'tags/',
  })

  .constant('API_BUILD_BARON', {
    BASE: '/plugin/buildbaron',
    TICKETS_BY_TASK_ID: 'created_tickets/{task_id}',
  })

  // Multi Page App User Interface routes
  .constant('MPA_UI', {
    BUILD_BY_ID: _.template('/build/{build_id}'),
    TASK_BY_ID: _.template('/task/{task_id}'),
  })
