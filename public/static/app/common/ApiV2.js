mciModule.factory('ApiV2', function(ApiUtil, API_V2) {
  var get = ApiUtil.httpGetter(API_V2.BASE)

  return {
    getPatchById: function(patchId) {
      return get(API_V2.PATCH_BY_ID, {patch_id: patchId})
    },

    getVersionById: function(versionId) {
      return get(API_V2.VERSION_BY_ID, {version_id: versionId})
    },

    // WATERFALL API
    getRecentVersions: function(projectId, getParms) {
      return get(API_V2.RECENT_VERSIONS, {project_id: projectId}, getParms)
    },
  }
})
