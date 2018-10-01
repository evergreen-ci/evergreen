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
  // :returns: none
  function applyMultiselectOptions(data, fields, gridApi) {
    // Create a column accessor function
    var colAccessor = getColAccessor(gridApi)

    // Apply options data to column options
    _.each(fields, function(d) {
      colAccessor(d).filter.options = _.chain(data)
        .pluck(d)
        .unique()
        .sortBy()
        .value()
    })
  }

  // Does row filtering for ui grid. List of params is compatible
  // with ui-grid condition function format
  // :param term: array of selected 'terms'
  // :returns: True if val is equal to at least one of the terms.
  //           False otherwise
  function multiselectConditionFn(term, val, row, col) {
    if (typeof(term) == 'undefined') return true
    if (term.length === 0) return true
    return _.contains(term, val)
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
