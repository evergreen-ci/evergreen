mciModule.controller('PerformanceDiscoveryCtrl', function(
  $q, $scope, $window, ApiTaskdata, ApiV1, ApiV2, EVG, EvgUiGridUtil,
  PERF_DISCOVERY, PerfDiscoveryDataService, PerfDiscoveryStateService,
  uiGridConstants
) {
  var vm = this;
  var gridUtil = EvgUiGridUtil
  var stateUtil = PerfDiscoveryStateService
  var dataUtil = PerfDiscoveryDataService
  var PD = PERF_DISCOVERY
  var grid
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

  dataUtil.getComparisionOptions(projectId)
    .then(function(items) {
      vm.fromSelect.options = items
      // Sets 'compare from' version from the state if available
      // Sets the first revision from the list otherwise
      var fromFound = dataUtil.findVersionItem(items, state.from)
      vm.fromSelect.selected = fromFound
        ? fromFound
        : _.findWhere(items, {kind: PD.KIND_VERSION})

      vm.toSelect.options = items
      // Sets 'compare to' version from the state if available
      // Sets the first tag from the list otherwise
      var toFound = dataUtil.findVersionItem(items, state.to)
      vm.toSelect.selected = toFound
        ? toFound
        : _.findWhere(items, {kind: PD.KIND_TAG})

      console.log(items)
      // Load grid data once revisions and tags loaded
      //vm.updateData()
    })

  // Handles changes in selectFrom/To drop downs
  // Ignores `null` on start up
  $scope.$watch('$ctrl.fromSelect.selected', function(item) {
    item && vm.updateData()
  })

  $scope.$watch('$ctrl.toSelect.selected', function(item) {
    item && vm.updateData()
  })

  vm.updateData = function() {
    // Set loading flag to display spinner
    vm.isLoading = true

    var fromVersion = vm.fromSelect.selected
    var toVersion = vm.toSelect.selected

    // Update permalink
    stateUtil.applyState(state, {
      from: fromVersion.id,
      to: toVersion.id,
    })

    // Display no data while loading is in progress
    vm.gridOptions.data = []

    // Depending on is revision patch id or version id
    // Different steps should be performed
    // Patch id requires additional step
    //var chain = fromVersion == PD.KIND_VERSION
    //  ? ApiV2.getPatchById(fromVersion.id)
    //  : $q.resolve(fromVersion.id)

    //chain
    //  // Get version by revision
    //  .then(function(revision) {
    //    return ApiV1.getVersionByRevision(projectId, revision)
    //  })
    console.log('UPDATE', fromVersion.id, toVersion.id)
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
      })
      // Stop spinner
      .finally(function() { vm.isLoading = false })
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
      gridApi.core.on.rowsRendered($scope, _.once(function() {
        stateUtil.applyStateToGrid(state, grid)
        // Set handlers after grid initialized
        gridApi.core.on.sortChanged(
          $scope, stateUtil.onSortChanged(state)
        )
        gridApi.core.on.filterChanged(
          $scope, stateUtil.onFilteringChanged(state, grid)
        )
      }))
    },
    columnDefs: [
      {
        name: 'Link',
        field: 'link',
        enableFiltering: false,
        width: 60,
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
        cellTemplate: '<micro-trend-chart data="COL_FIELD" />',
        width: PERF_DISCOVERY.TREND_COL_WIDTH,
        enableSorting: false,
        enableFiltering: false,
      },
      {
        name: 'Avg and Self',
        field: 'avgVsSelf',
        cellTemplate: '<micro-trend-chart data="COL_FIELD" />',
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
