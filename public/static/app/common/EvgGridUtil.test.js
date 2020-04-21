describe('PerfDiscoveryServiceTest', function() {
  beforeEach(module('MCI'));

  var gridUtil

  beforeEach(inject(function($injector) {
    gridUtil = $injector.get('EvgUiGridUtil')
  }))

  it('returns col object by col name', function() {
    var colB = {field: 'b'}
    var gridApi = {
      grid: {
        columns: [{field: 'a'}, colB],
      },
    }

    expect(
      gridUtil.getColAccessor(gridApi)('b')
    ).toBe(colB)
  })

  it('applies multiselect options to col filters', function() {
    var data = [
      {a: 'a1', b: 'b1'},
      {a: 'a2', b: 'b2'},
      {a: 'a2', b: 'b2'}, // duplicate
    ]
    var fields = ['a']
    var gridApi = {
      grid: {
        columns: [
          {field: 'a', filter: {options: []}},
          {field: 'b', filter: {options: []}},
        ],
      },
    }

    gridUtil.applyMultiselectOptions(data, fields, gridApi)

    expect(
      gridApi.grid.columns[0].filter.options
    ).toEqual(['a1', 'a2'])

    expect(
      gridApi.grid.columns[1].filter.options
    ).toEqual([])
  })

  it('filter items using OR condition', function() {
    expect(gridUtil.multiselectConditionFn(undefined, 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn([], 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn(['a'], 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn(['b', 'a', 'c'], 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn(['b', 'c'], 'a')).toBe(false)
  }) 

  it('filter items with exclusionary filtering', function() {
    expect(gridUtil.multiselectConditionFn(['!a', '!b'], 'X')).toBe(true)
    expect(gridUtil.multiselectConditionFn(['!a', '!b'], 'a')).toBe(false)
    expect(gridUtil.multiselectConditionFn(['!a', '!b'], 'b')).toBe(false)
  })

  it('filts items with mixed [ex|in]clusionary filtering', function() {
    expect(gridUtil.multiselectConditionFn(['a', '!b'], 'X')).toBe(false)
    expect(gridUtil.multiselectConditionFn(['a', '!b'], 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn(['a', '!b'], 'b')).toBe(false)

    expect(gridUtil.multiselectConditionFn(['b', '!b'], 'b')).toBe(false)
    expect(gridUtil.multiselectConditionFn(['b', '!b'], 'X')).toBe(false)
  })

  it('filts items with mixed [ex|in]clusionary filtering (reversed order)', function() {
    expect(gridUtil.multiselectConditionFn(['!b', 'a'], 'X')).toBe(false)
    expect(gridUtil.multiselectConditionFn(['!b', 'a'], 'a')).toBe(true)
    expect(gridUtil.multiselectConditionFn(['!b', 'a'], 'b')).toBe(false)

    expect(gridUtil.multiselectConditionFn(['!b', 'b'], 'b')).toBe(false)
    expect(gridUtil.multiselectConditionFn(['!b', 'b'], 'X')).toBe(false)
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

  function createRow(value) {
    return {
      entity: {
        order: value
      }
    };
  }

  it('should return 0 if values are equal', () => {
    const value = gridUtil.sortByOrder(5, 5);
    expect(value).toEqual(0);
  });

  it('should return 0 if order values are equal', () => {
    const value = gridUtil.sortByOrder(3, 5, createRow(5), createRow(5));
    expect(value).toEqual(0);
  });

  it('should return negative if row a is less than row b', () => {
    const value = gridUtil.sortByOrder(3, 5, createRow(3), createRow(5));
    expect(value).toBeLessThan(0);
  });

  it('should return positive if row a is greater than row b', () => {
    const value = gridUtil.sortByOrder(3, 5, createRow(5), createRow(3));
    expect(value).toBeGreaterThan(0);
  });

  it('should sort by revision if rows are not available', () => {
    const value = gridUtil.sortByOrder(3, 5, createRow(5));
    expect(value).toBeLessThan(0);
  });
})
