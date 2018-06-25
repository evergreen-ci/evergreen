mciModule.controller('PerformanceDiscoveryCtrl', function(
  $q, $scope, $timeout, $window, ApiTaskdata, ApiV1, ApiV2,
  EVG, EvgUiGridUtil, PERF_DISCOVERY, PerfDiscoveryDataService,
  PerfDiscoveryStateService, uiGridConstants 
) {
  var vm = this;
  var gridUtil = EvgUiGridUtil
  var stateUtil = PerfDiscoveryStateService
  var dataUtil = PerfDiscoveryDataService
  var PD = PERF_DISCOVERY
  var grid
  vm.refCtx = [0, 0]
  vm.jiraHost = $window.JiraHost

  // Load state from the URL
  var state = stateUtil.readState({
    // Default sorting
    sort: {
      ratio: {
        priority: 0,
        direction: uiGridConstants.DESC,
      }
    }
  })

  vm.fromSelect = {
    options: [],
    selected: null,
  }

  vm.toSelect = {
    options: [],
    selected: null,
  }

  var projectId = $window.project

  // For each argument of `arguments`
  // if arguemnt is a function and return value is truthy
  // or argument is truthy non-function return the value
  // If none arguments are truthy returns undefined
  function cascade() {
    var args = Array.prototype.slice.call(arguments)
    for (var i = 0; i < args.length; i++) {
      var ref = args[i]
      var value = _.isFunction(ref) ? ref() : ref
      if (value) return value
    }
  }

  dataUtil.getComparisionOptions(projectId)
    .then(function(items) {
      vm.fromSelect.options = items

      // Sets 'compare from' version from the state if available
      // Sets the first revision from the list otherwise
      vm.fromSelect.selected = cascade(
        _.bind(dataUtil.findVersionItem, null, items, state.from),
        _.bind(dataUtil.getQueryBasedItem, null, state.from),
        _.bind(_.findWhere, null, items, {kind: PD.KIND_VERSION}),
        _.bind(_.first, null, items)
      )

      vm.toSelect.options = items
      // Sets 'compare to' version from the state if available
      // Sets the first tag from the list otherwise
      vm.toSelect.selected = cascade(
        _.bind(dataUtil.findVersionItem, null, items, state.to),
        _.bind(dataUtil.getQueryBasedItem, null, state.to),
        _.bind(_.findWhere, null, items, {kind: PD.KIND_TAG}),
        _.bind(_.first, null, items)
      )
    })

  // Handles changes in selectFrom/To drop downs
  // Ignores `null` on start up
  $scope.$watch('$ctrl.fromSelect.selected', function(item) {
    item && vm.updateData()
  })

  $scope.$watch('$ctrl.toSelect.selected', function(item) {
    item && vm.updateData()
  })

  var oldFromVersion, oldToVersion

  function loadCompOptions(fromVersion, toVersion) {
    // Set loading flag to display spinner
    vm.isLoading = true
  
    $q.all({
      fromVersionObj: dataUtil.getCompItemVersion(fromVersion),
      toVersionObj: dataUtil.getCompItemVersion(toVersion),
    })
      // Load perf data
      .then(function(promise) {
        return dataUtil.getData(
          promise.fromVersionObj, promise.toVersionObj
        )
      })
      // Apply perf data
      .then(function(res) {
        vm.gridOptions.data = res
        // Apply options data to filter drop downs
        gridUtil.applyMultiselectOptions(
          res,
          ['build', 'storageEngine', 'task', 'threads'],
          vm.gridOptions
        )
        return res
      })
      // Stop spinner
      .finally(function() { vm.isLoading = false })
      // Fetch BF tickets
      .then(function() {
        return dataUtil.getBFTicketsForRows(vm.gridOptions.data)
      })
      // Apply BF tickets to rows data
      .then(function(bfGroups) {

        // Match grouped points (by task, bv, test) w/ rows
        _.each(bfGroups, function(bfGroup) {
          // Dedupe by 'key' (BFs might have different revisions or other fields)
          var bfs = _.uniq(bfGroup, function(d) { return d.key })

          var bf = bfs[0] // using the first element as characteristic

          var matcher = {
            task: bf.tasks,
            buildVariant: bf.buildvariants,
          }

          // If BF has associated test
          bf.tests && (matcher['test'] = bf.tests)

          _.each(_.where(vm.gridOptions.data, matcher), function(task) {
            task.buildFailures = bfs
          })
        })
      })
  }

  vm.updateData = function() {
    var fromVersion = vm.fromSelect.selected
    var toVersion = vm.toSelect.selected

    // If nothing has changed, exit the function
    if (fromVersion == oldFromVersion && toVersion == oldToVersion) {
      return
    }

    oldFromVersion = fromVersion
    oldToVersion = toVersion

    // Update permalink
    stateUtil.applyState(state, {
      from: fromVersion.id,
      to: toVersion.id,
    })

    // Display no data while loading is in progress
    vm.gridOptions.data = []

    loadCompOptions(fromVersion, toVersion)
  }

  function updateChartContext(grid) {
    // Update context chart data for given rendered rows
    // sets [min, max] list to the scope for visible rows
    vm.refCtx = d3.extent(
      _.reduce(grid.renderContainers.body.renderedRows, function(m, d) {
        return m.concat([
          Math.log(d.entity.avgVsSelf[0]),
          Math.log(d.entity.avgVsSelf[1])
        ])
      }, [])
    )
  }

  // Returns a predefined URL for given `row` and `col`
  // Works with build abd task columns only
  vm.getCellUrl = function(row, col) {
    return row.entity[{
      build: 'buildURL',
      task: 'taskURL',
    }[col.field]]
  }

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    onRegisterApi: function(gridApi) {
      grid = gridApi.grid;

      // Using _.once, because this behavior is required on init only
      gridApi.core.on.rowsRendered($scope, function() {
        // For some reason, calback being called before
        // the changes were applied to grid
        // Timeout forces underlying code to be executed at the end
        $timeout(
          _.bind(updateChartContext, null, grid) // When rendered, update charts context
        )
      })

      gridApi.core.on.rowsRendered($scope, _.once(function() { // Do once
        stateUtil.applyStateToGrid(state, grid)
        // Set handlers after grid initialized
        gridApi.core.on.sortChanged(
          $scope, stateUtil.onSortChanged(state)
        )
        gridApi.core.on.filterChanged(
          $scope, stateUtil.onFilteringChanged(state, grid)
        )
      }))

      // Adding grid to $scope to create a watcher
      $scope.grid = grid
      // This triggers on vertical scroll
      $scope.$watch(
        'grid.renderContainers.body.currentTopRow', function() {
          updateChartContext(grid)
        }
      )
    },
    columnDefs: [
      {
        name: 'Tickets',
        field: 'buildFailures',
        cellTemplate: 'perf-discovery-bfs',
        width: 100,
      },
      gridUtil.multiselectColDefMixin({
        name: 'Build',
        field: 'build',
        cellTemplate: 'perf-discovery-link',
      }),
      gridUtil.multiselectColDefMixin({
        name: 'Storage Engine',
        field: 'storageEngine',
      }),
      gridUtil.multiselectColDefMixin({
        name: 'Task',
        field: 'task',
        cellTemplate: 'perf-discovery-link',
      }),
      {
        name: 'Test',
        field: 'test',
      },
      gridUtil.multiselectColDefMixin({
        name: 'Threads',
        field: 'threads',
        type: 'number',
        width: 130,
      }),
      {
        name: 'Ratio',
        field: 'ratio',
        type: 'number',
        cellTemplate: '<perf-discovery-ratio ratio="COL_FIELD" />',
        enableFiltering: false,
        width: 80,
      },
      {
        name: 'Average Ratio',
        field: 'avgRatio',
        type: 'number',
        cellTemplate: '<perf-discovery-ratio ratio="COL_FIELD"/>',
        enableFiltering: false,
        visible: false,
        width: 80,
      },
      {
        name: 'Trend',
        field: 'trendData',
        cellTemplate: '<micro-trend-chart data="COL_FIELD" ctx="grid.appScope.$ctrl.refCtx"/>',
        width: PERF_DISCOVERY.TREND_COL_WIDTH,
        enableSorting: false,
        enableFiltering: false,
      },
      {
        name: 'Avg and Self',
        field: 'avgVsSelf',
        cellTemplate: '<micro-trend-chart data="COL_FIELD" ctx="grid.appScope.$ctrl.refCtx"/>',
        width: PERF_DISCOVERY.TREND_COL_WIDTH,
        enableSorting: false,
        enableFiltering: false,
      },
      {
        name: 'ops/sec',
        field: 'speed',
        type: 'number',
        cellFilter: 'number:2',
        enableFiltering: false,
        width: 100,
      },
      {
        name: 'Baseline',
        field: 'baseSpeed',
        type: 'number',
        cellFilter: 'number:2',
        enableFiltering: false,
        width: 120,
      },
    ]
  }
})
