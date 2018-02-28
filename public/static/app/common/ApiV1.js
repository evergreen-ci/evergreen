mciModule.factory('ApiV1', function($http, ApiUtil, API_V1) {
  var get = ApiUtil.httpGetter(API_V1.BASE)

  return {
    getProjectVersions: function(projectId) {
      return get(API_V1.PROJECTS_VERSION_API, {project_id: projectId})
    },

    getProjectDetail: function(projectId) {
      return get(API_V1.PROJECTS_DETAIL_API, {project_id: projectId})
    },

    getBuildDetail: function(buildId) {
      return get(API_V1.BUILDS_DETAIL, {build_id: buildId})
    },

    getBuildStatus: function(buildId) {
      return get(API_V1.BUILDS_STATUS, {build_id: buildId})
    },

    getVersionByRevision: function(projectId, revision) {
      return get(API_V1.PROJECTS_REVISIONS_API, {
        project_id: projectId,
        revision: revision,
      })
    },

    // VERSIONS API
    getVersionById: function(versionId) {
      return get(API_V1.VERSIONS_BY_ID, {version_id: versionId})
    },

    // WATERFALL API
    getWaterfallVersionsRows: function(projectId, getParms) {
      return get(API_V1.WATERFALL_VERSIONS_ROWS, {project_id: projectId}, getParms)
    },
  }
})
