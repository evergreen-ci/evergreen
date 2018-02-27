describe('PerfDiscoveryServiceTest', function() {
  beforeEach(module('MCI'));

  var gridUtil

  beforeEach(inject(function($injector) {
    gridUtil = $injector.get('EvgUiGridUtil')
  }))

  it('returns col object by col name', function() {
    var colB = {field: 'b'}
    var gridOptions = {
      columnDefs: [{field: 'a'}, colB]
    }

    expect(
      gridUtil.getColAccessor(gridOptions)('b')
    ).toBe(colB)
  })

  it('applies multiselect options to col filters', function() {
    var data = [
      {a: 'a1', b: 'b1'},
      {a: 'a2', b: 'b2'},
      {a: 'a2', b: 'b2'}, // duplicate
    ]
    var fields = ['a']
    var gridOptions = {
      columnDefs: [
        {field: 'a', filter: {options: []}},
        {field: 'b', filter: {options: []}},
      ]
    }

    gridUtil.applyMultiselectOptions(data, fields, gridOptions)

    expect(
      gridOptions.columnDefs[0].filter.options
    ).toEqual(['a1', 'a2'])

    expect(
      gridOptions.columnDefs[1].filter.options
    ).toEqual([])
  })

  it('filter items using OR condition', function() {
    expect(gridUtil.multiselectConditionFn(undefined, 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn([], 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn(['a'], 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn(['b', 'a', 'c'], 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn(['b', 'c'], 'a')).toBe(false)
  }) 

  it('extends col def with multiselect filter boilerplate', function() {
    var col = gridUtil.multiselectColDefMixin({
      field: 'a',
      filter: {noTerm: false} //override mixin
    })

    expect(col.filterHeaderTemplate).toBeDefined()
    expect(col.filter).toBeDefined()
    expect(col.filter.noTerm).toBe(false)
  })
})
