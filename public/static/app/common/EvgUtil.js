mciModule.factory('EvgUtil', function() {
  var DATE_ID_FORMAT = 'YY_MM_DD_HH_mm_ss'
  var GENERIC_ID_RE = /([\w\.]+)_([a-z0-9]+)_(\d{2})_(\d{2})_(\d{2})_(\d{2})_(\d{2})_(\d{2})$/

  // Replaces all '-' with '_'
  function normalize(text) {
    return text.replace(/-/g, '_')
  }

  // internal function which generates evergreen id from given tokens
  function generateId(tokens) {
    return _.map(tokens, normalize).join('_')
  }

  // Parses Evergreen ID and returns the result
  // :returns: {
  //   ok: true|false, // parse status
  //   head: str, // project/build/test/tasl cannot be parsed
  //   revision: str(40),
  //   createTime: moment.utc time,
  // }
  function parseId(id) {
    var part = (id || '').match(GENERIC_ID_RE)
    var ok = Boolean(part)

    return {
      ok: ok,
      head: ok ? part[1] : '',
      revision: ok ? part[2] : '',
      // Evergreen ID has zero-indexed montheh.
      // In combination with moment API it parses dates correctly
      createTime: ok ? moment.utc([
        +('20' + part[3]), part[4] - 1, +part[5], // Date part
        +part[6], +part[7], +part[8] // Time part
      ]) : moment(0),
    }
  }

  return {
    // Generates version id from project_id and revision
    // :param params: {project, revision, buildVariant, createTime}
    generateBuildId: function (params) {
      return generateId([
        params.project,
        params.buildVariant,
        params.revision,
        moment.utc(params.createTime).format(DATE_ID_FORMAT)
      ])
    },

    // Generates version id from project_id and revision
    // :param params: {project, revision}
    generateVersionId: function (params) {
      return generateId([params.project, params.revision])
    },

    parseId: parseId,
  }
})
