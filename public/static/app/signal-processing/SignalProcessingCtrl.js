mciModule.controller('SignalProcessingCtrl', function(
  $window, $scope, MDBQueryAdaptor, Stitch, FORMAT, STITCH_CONFIG
) {
  var vm = this;

  // TODO later this might be replaced with some sort of pagination
  var LIMIT = 500

  vm.mode = {
    options: [{
      id: 'processed',
      name: 'Processed',
    }, {
      id: 'unprocessed',
      name: 'Unprocessed',
    }],
    value: 'unprocessed',
  }

  var state = {
    sorting: [{
      field: 'suspect_revision',
      direction: 'asc',
    }],
    filtering: {
      create_time: '>' + moment().subtract(2, 'weeks').format(FORMAT.ISO_DATE),
      probability: '>0.05',
      project: '=' + $window.project,
    },
    mode: vm.mode.value,
  }

  var modeToCollMap = {
    unprocessed: STITCH_CONFIG.PERF.COLL_UNPROCESSED_POINTS,
    processed: STITCH_CONFIG.PERF.COLL_PROCESSED_POINTS,
  }

  // Required by loadData.
  var theMostRecentPromise

  function loadData(state) {
    vm.isLoading = true
    theMostRecentPromise = Stitch.use(STITCH_CONFIG.PERF).query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(modeToCollMap[state.mode])
        .aggregate(getAggChain(state))
    })
    // Storing this promise in closure.
    var thisPromise = theMostRecentPromise
    thisPromise.then(function(docs) {
      // There more than one concurring promises - we want the most recent one
      if (thisPromise != theMostRecentPromise) {
        return
      }
      theMostRecentPromise
        .then(function() {
          vm.gridOptions.data = docs
        }, function(err) {
          console.error(err)
        }).finally(function() {
          vm.isLoading = false
        })
    })
  }

  // Helper function which returns `col` for given `gridApi` and `colName`
  function getCol(gridApi, colName) {
    return _.findWhere(gridApi.grid.columns, {field: colName})
  }

  // Enhances filtering state with some contextual meta data
  // This data is required by expression compiler
  function getFilteringContext(state) {
    return _.reduce(state.filtering, function(m, v, k) {
      var col = getCol(vm.gridApi, k)
      if (!col) return m // Error! Associated col does not found
      return m.concat({
        field: k,
        term: v,
        type: col.colDef.type || 'string',
      })
    }, [])
  }

  // Creates aggregation expression, which could be used by Stitch
  // for given `state`
  function getAggChain(state) {
    var chain = []

    // Check if the state has filtering
    if (!_.isEmpty(state.filtering)) {
      var filteringChain = MDBQueryAdaptor.compileFiltering(
        // filtering context enhaces state data with important meta data
        getFilteringContext(state)
      )
      // check if filtering query was compiled into something
      filteringChain && chain.push(filteringChain)
    }

    if (state.sorting) {
      var sortingChain = MDBQueryAdaptor.compileSorting(state.sorting)
      // check if sorting query was compiled into something
      sortingChain && chain.push(sortingChain)
    }

    chain.push({$limit: LIMIT})
    return chain
  }

  vm.modeChanged = function() {
    state.mode = vm.mode.value
    loadData(state)
  }

  // Sets `state` to grid filters
  function setInitialGridState(gridApi, state) {
    _.each(state.filtering, function(term, colName) {
      var col = getCol(vm.gridApi, colName)
      if (!col) return // Error! Associated col does not found
      col.filters = [{term: term}]
    })

    _.each(state.sorting, function(sortingItem) {
      var col = getCol(vm.gridApi, sortingItem.field)
      if (!col) return // Error! Associated col does not found
      col.sort.direction = sortingItem.direction
    })
  }

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    useExternalFiltering: true,
    useExternalSorting: true,
    onRegisterApi: function(api) {
      vm.gridApi = api
      api.core.on.sortChanged($scope, function(grid, cols) {
        state.sorting = _.map(cols, function(col) {
          return {
            field: col.field,
            direction: col.sort.direction
          }
        })
        loadData(state)
      })

      var onFilterChanged = _.debounce(function() {
        state.filtering = _.reduce(api.grid.columns, function(m, d) {
          var term = d.filters[0].term
          if (term) m[d.field] = term
          return m
        }, {})
        loadData(state)
      }, 200)

      api.core.on.filterChanged($scope, onFilterChanged)

      // Load intial set of data once `columns` are populated
      api.core.on.rowsRendered(null, _.once(function() {
        setInitialGridState(api, state)
        loadData(state)
      }))
    },
    columnDefs: [
      {
        // TODO Jim: Should be managed by PERF-1546
        name: 'Hazard Level',
        cellTemplate: '<hazard-level-cell row="row" />',
      },
      {
        name: 'Variant',
        field: 'variant',
        type: 'string',
      },
      {
        name: 'Task',
        field: 'task',
        type: 'string',
      },
      {
        name: 'Test',
        field: 'test',
        type: 'string',
      },
      {
        name: 'Revision',
        field: 'suspect_revision',
        type: 'string',
        sort: {
          priority: 0,
        },
        cellTemplate: 'ui-grid-group-name',
        grouping: {
          groupPriority: 0,
        },
      },
      {
        name: 'Value',
        field: 'value',
        cellFilter: 'number:2',
        type: 'number',
      },
      {
        name: 'Value to Avg',
        field: 'value_to_avg',
        cellFilter: 'number:2',
        type: 'number',
      },
      {
        name: 'Probability',
        field: 'probability',
        cellFilter: 'number:2',
        type: 'number',
      },
      {
        name: 'Average',
        field: 'average',
        cellFilter: 'number:2',
        visible: false,
        type: 'number',
      },
      {
        name: 'Average Diff',
        field: 'average_diff',
        cellFilter: 'number:2',
        visible: false,
        type: 'number',
      },
      {
        name: 'Value to Avg Diff',
        field: 'value_to_avg_diff',
        cellFilter: 'number:2',
        visible: false,
        type: 'number',
      },
      {
        name: 'Processed Type',
        field: 'processed_type',
        visible: false,
      },
      {
        name: 'Thread Level',
        field: 'thread_level',
        visible: false,
        type: 'number',
      },
      {
        name: 'Create Time',
        field: 'create_time',
        visible: false,
        type: 'date',
      },
      {
        name: 'Project',
        field: 'project',
        type: 'string',
        visible: false,
      },
    ]
  }
})
