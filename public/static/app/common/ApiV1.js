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
  }
})
