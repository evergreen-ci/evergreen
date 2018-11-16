mciModule.factory('EvgUiGridUtil', function() {
  // Allows to get ui grid col options
  // :param gridOptions: ui grid api
  // :returns: Function (with gridApi stored in the context)
  function getColAccessor(gridApi) {
    // :param fieldName: ui grid field name
    // :returns: ui grid col object
    return function(fieldName) {
      return _.findWhere(gridApi.grid.columns, {
        field: fieldName
      })
    }
  }

  // Applies filter options to ui grid columns
  // Options are unique set of unique items for each given field
  // :param field: List of strings (field names)
  // :param gridAou: ui grid api
  // :param exclusionaryMode: also adds the same items prefixed with !
  // :returns: none
  function applyMultiselectOptions(data, fields, gridApi, exclusionaryMode) {
    // Create a column accessor function
    var colAccessor = getColAccessor(gridApi)

    // Apply options data to column options
    _.each(fields, function(d) {
      var opts = _.chain(data)
        .pluck(d)
        .unique()
        .sortBy()
        .value()

      // Adds items prefixed with !
      if (exclusionaryMode) {
        opts = opts.concat(_.map(opts, function(d) { return '!' + d }))
      }

      colAccessor(d).filter.options = opts
    })
  }

  // Compiles `terms` into list of predicate functions and operators
  // :param terms: [String, ...], any term could be prefixed with ! for negation
  // :returns: [{predicate: String -> Boolean, op: 'AND'|'OR'}, ...]
  function compilePredicates(terms) {
    return _.chain(terms)
       .map(function(termExpr) {
        if (termExpr[0] == '!') {
          var term = termExpr.substr(1)
          return {
            predicate: function(value) { return term != value },
            op: 'AND',
          }
        } else {
          return {
            predicate: function(value) { return termExpr == value },
            op: 'OR',
          }
        }
      })
      .sortBy('op') // OR predicates should go first
      .reverse()
      .value()
  }

  // Store compiled predicates in cache, to prevent recompilation for each row
  var termsCacheKey, predicatesCache

  // Does row filtering for ui grid. List of params is compatible
  // with ui-grid condition function format
  // :param terms: array of selected 'terms'
  // :returns: True if val is equal to at least one of the terms.
  //           False otherwise
  function multiselectConditionFn(terms, val, row, col) {
    if (typeof(terms) == 'undefined') return true
    if (terms.length === 0) return true

    // Check if we already have comipler predicates for given `terms`
    if (terms != termsCacheKey) {
      termsCacheKey = terms
      predicatesCache = compilePredicates(terms)
    }

    // Apply prediicates and operators
    // Probably, a bit time consuming (Nrows * Mpredicates)
    // Ideally, I would like compile all predicates to simple sort of reg exp
    return _.reduce(_.rest(predicatesCache), function (m, d) {
      if (d.op == 'AND') {
        return m && d.predicate(val)
      } // else if 'OR'
      return m || d.predicate(val)
    }, _.first(predicatesCache).predicate(val))
  }

  // Extends ui grid column definition with multiselect filter
  // boilerplate code. Custom `colDef` options have the highest
  // priority
  // :param colDef: ui grid column definition
  // :returns: ui grid column definition (with boilerplate)
  function multiselectColDefMixin(colDef) {
    return _.extend({}, {
      filterHeaderTemplate: 'evg-ui-select/header-filter',
      filter: {
        noTerm: true,
        condition: multiselectConditionFn,
        options: [],
        mode: 'multi',
      }
    }, colDef)
  }

  return {
    applyMultiselectOptions: applyMultiselectOptions,
    getColAccessor: getColAccessor,
    multiselectColDefMixin: multiselectColDefMixin,
    multiselectConditionFn: multiselectConditionFn,
  }
})
