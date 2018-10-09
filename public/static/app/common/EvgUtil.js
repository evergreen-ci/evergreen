mciModule.factory('EvgUtil', function() {
  var DATE_ID_FORMAT = 'YY_MM_DD_HH_mm_ss'

  // Replaces all '-' with '_'
  function normalize(text) {
    return text.replace(/-/g, '_')
  }

  // internal function which generates evergreen id from given tokens
  function generateId() {
    return _.map(Array.prototype.slice.call(arguments), normalize).join('_')
  }

  return {
    // Generates version id from project_id and revision
    // :param params: {project, revision, buildVariant, createTime}
    generateBuildId: function (params) {
      return generateId(
        params.project,
        params.buildVariant,
        params.revision,
        moment.utc(params.createTime).format(DATE_ID_FORMAT)
      )
    },

    // Generates version id from project_id and revision
    // :param params: {project, revision}
    generateVersionId: function (params) {
      return generateId(params.project, params.revision)
    },
  }
})
