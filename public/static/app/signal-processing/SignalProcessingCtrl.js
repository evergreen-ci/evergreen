mciModule.controller('SignalProcessingCtrl', function(
  $window, $scope, Stitch, STITCH_CONFIG
) {
  var vm = this;
  var projectId = $window.project

  var state = {
    sorting: null,
    filtering: {}
  }

  function loadData(state) {
    vm.isLoading = true
    Stitch.use(STITCH_CONFIG.PERF).query(function(db) {
      return db
        .db(STITCH_CONFIG.PERF.DB_PERF)
        .collection(STITCH_CONFIG.PERF.COLL_CHANGE_POINTS)
        .aggregate(getAggChain(state))
    }).then(function(docs) {
      vm.gridOptions.data = docs
    }, function(err) {
      console.error(err)
    }).finally(function() {
      vm.isLoading = false
    })
  }

  loadData(state)

  function getAggChain(state) {
    var chain = []

    // Desctruct.
    var sorting = state.sorting
    var filtering = state.filtering

    if (!_.isEmpty(filtering)) {
      chain.push(mdbFiltering(filtering))
    }

    if (sorting) {
      chain.push(mdbSorting(sorting))
    }

    chain.push({$limit: 250})
    return chain
  }

  function mdbFiltering(filtering) {
      return {
        $match: _.mapObject(filtering, function(d) {
          return {$regex: d, $options: 'i'}
        })
      }
  }

  function mdbSorting(sorting) {
    var q = {$sort: {}}
    q.$sort[sorting.field] = sorting.direction == 'asc' ? 1 : -1
    return q
  }

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    useExternalFiltering: true,
    useExternalSorting: true,
    onRegisterApi: function(api) {
      api.core.on.sortChanged($scope, function(grid, cols) {
        state.sorting = {field: cols[0].field, direction: cols[0].sort.direction}
        loadData(state)
      })

      var onFilterChanged = _.debounce(function() {
        state.filtering = _.reduce(api.grid.columns, function(m, d) {
          var term = d.filters[0].term
          if (term) m[d.field] = term
          return m
        }, {})
        loadData(state)
      }, 100)

      api.core.on.filterChanged($scope, onFilterChanged)
    },
    columnDefs: [
      {
        name: 'Project',
        field: 'project',
      },
      {
        name: 'Variant',
        field: 'variant',
      },
      {
        name: 'Task',
        field: 'task',
      },
      {
        name: 'Test',
        field: 'test',
      },
      {
        name: 'Revision',
        field: 'revision',
      },
      {
        name: 'Value',
        field: 'value',
        cellFilter: 'number:2',
        enableFiltering: false,
      },
      {
        name: 'Value to Avg',
        field: 'value_to_avg',
        cellFilter: 'number:2',
        enableFiltering: false,
      },
      {
        name: 'Average',
        field: 'average',
        cellFilter: 'number:2',
        enableFiltering: false,
        visible: false,
      },
      {
        name: 'Average Diff',
        field: 'average_diff',
        cellFilter: 'number:2',
        enableFiltering: false,
        visible: false,
      },
      {
        name: 'Value to Avg Diff',
        field: 'value_to_avg_diff',
        cellFilter: 'number:2',
        enableFiltering: false,
        visible: false,
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
      },
      {
        name: 'Create Time',
        field: 'create_time',
        visible: false,
      },
    ]
  }
})
