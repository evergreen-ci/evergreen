mciModule.factory('PerfDiscoveryStateService', function(
  $httpParamSerializer, $location
) {
  /* STATE ADAPTORS */

  function sortToState(cols) {
    return {
      sort: _.reduce(cols, function(m, d) {
        m[d.field] = d.sort
        return m
      }, {})
    }
  }

  function onSortChanged(state) {
    return function(grid, cols) {
      applyState(state, sortToState(cols))
    }
  }

  function filteringToState(cols) {
    return {
      filtering: _.reduce(cols, function(m, d) {
        m[d.field] = d.filters[0].term
        return m
      }, {})
    }
  }

  function onFilteringChanged(state, grid) {
    return function() {
      applyState(
        state,
        filteringToState(
          _.filter(
            grid.columns, function(col) { return col.filters[0].term }
          )
        )
      )
    }
  }

  /* SERIALIZATION */

  function serializeFromVersion(state) {
    return {from: state.from}
  }

  function serializeToVersion(state) {
    return {to: state.to}
  }

  function serealizeSorting(state) {
    return {
      sort: _.map(state.sort, function(v, k) {
        return (v.direction == 'desc' ? '-' : '') + k + v.priority
      })
    }
  }

  function serealizeFiltering(state) {
    return _.reduce(state.filtering, function(m, v, k) {
      m[k] = v
      return m
    }, {})
  }

  /* DESERIALIZATION */

  function deserializeFromVersion(serialized) {
    var from = serialized.from
    return from ? {from: from} : {}
  }

  function deserializeToVersion(serialized) {
    var to = serialized.to
    return to ? {to: to} : {}
  }

  function deserializeSorting(serialized) {
    var re = /(-?)(\w+)(\d)/
    var DIRECTION = 1
    var FIELD = 2
    var PRIORITY = 3
    // normalize
    var val = serialized.sort
    var sorting = val != undefined && !_.isArray(val) ? [val] : val

    var sorting = _.reduce(sorting, function(m, d) {
      var parts = d.match(re)
      m[parts[FIELD]] = {
        direction: parts[DIRECTION] == '-' ? 'desc' : 'asc',
        priority: +parts[PRIORITY],
      }
      return m
    }, {})

    // Don't add {sort} entry if no options available
    return !_.isEmpty(sorting) ? {sort: sorting} : {}
  }

  function deserializeFiltering(serialized) {
    var filtering = _.omit(serialized, 'sort', 'from', 'to')
    // Don't add {filtering} entry if no options available
    return !_.isEmpty(filtering) ? {filtering: filtering} : {}
  }

  /* UTILITY */

  function applyStateToGrid(state, grid) {
    var sortedFields = _.keys(state.sort)
    var filteredFields = _.keys(state.filtering)

    _.each(grid.columns, function(col) {
      if (_.includes(sortedFields, col.field)) {
        col.sort = state.sort[col.field]
      }

      if (_.includes(filteredFields, col.field)) {
        var filter = col.filters[0]
        var value = state.filtering[col.field]
        var term
        if (filter.mode == 'multi' && !_.isArray(value)) {
          term = [value]
        } else {
          term = value
        }

        if (col.colDef.type == 'number') {
          filter.term = _.map(term, function(d) { return +d })
        } else {
          filter.term = term
        }
      }
    })
  }

  function readState(state) {
    var params = $location.search()
    return _.extend(
      state,
      deserializeFiltering(params),
      deserializeSorting(params),
      deserializeFromVersion(params),
      deserializeToVersion(params)
    )
  }

  function applyState(state, statePatch) {
    _.extend(state, statePatch)
    var serialized = _.extend(
      serealizeSorting(state),
      serealizeFiltering(state),
      serializeFromVersion(state),
      serializeToVersion(state)
    )
    $location.search($httpParamSerializer(serialized))
  }

  return {
    // Public API
    readState: readState,
    applyState: applyState,
    applyStateToGrid: applyStateToGrid,
    onSortChanged: onSortChanged,
    onFilteringChanged: onFilteringChanged,
  }
})
