mciModule.factory('EvgUiGridUtil', function() {
  // Allows to get ui grid col options
  // :parm gridOptions: ui grid options
  // :returns: Function (with gridOptions stored in the context)
  function getColAccessor(gridOptions) {
    // :param fieldName: ui grid field name
    // :returns: ui grid col object
    return function(fieldName) {
      return _.findWhere(gridOptions.columnDefs, {
        field: fieldName
      })
    }
  }

  // Applies filter options to ui grid columns
  // Options is unique set of unique items for each given field
  // :param field: List of strings (field names)
  // :param gridOptions: ui grid options object
  // :returns: none
  function applyMultiselectOptions(data, fields, gridOptions) {
    // Create a column accessor function
    var colAccessor = getColAccessor(gridOptions)

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
        options: []
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
