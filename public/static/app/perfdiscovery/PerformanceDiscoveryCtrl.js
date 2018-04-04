mciModule.controller('PerformanceDiscoveryCtrl', function(
  $q, $scope, $window, ApiTaskdata, ApiV1, ApiV2, EVG, EvgUiGridUtil,
  PERF_DISCOVERY, PerfDiscoveryDataService, PerfDiscoveryStateService,
  uiGridConstants
) {
  var vm = this;
  var gridUtil = EvgUiGridUtil
  var stateUtil = PerfDiscoveryStateService
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

  vm.revisionSelect = {
    options: [],
    selected: null,
  }

  vm.tagSelect = {
    options: [],
    selected: null,
  }

  var projectId = $window.project

  // This function is used to make possible to type arbitrary
  // version revision into version drop down
  vm.getVersionOptions = function(query) {
    var opts = vm.revisionSelect.options
    // 24 is patch id length; 40 is githash length
    // don't allow user type invalid version identifier
    var isValid = _.contains(
      [EVG.GIT_HASH_LEN, EVG.PATCH_ID_LEN], query.length
    )

    if (isValid && opts.indexOf(query) == -1) {
      return opts.concat(query)
    }

    return opts
  }

  var versionsQ = ApiV2.getRecentVersions(projectId)
  var whenQueryRevisions = versionsQ.then(function(res) {
    vm.versions = _.map(
      _.where(
        res.data.versions, {rolled_up: false} // Filter versions with data
      ),
      function(d) { return {revision: d.revisions[0]} } // Transform
    )

    vm.revisionSelect.options = _.map(vm.versions.versions, function(d) {
      return d.revision
    })

    // Sets 'compare from' version from the state if available
    // Sets the first revision from the list otherwise
    vm.revisionSelect.selected = state.from
      ? state.from
      : _.first(vm.revisionSelect.options)
  })

  var whenQueryTags = ApiTaskdata.getProjectTags(projectId).then(function(res){
    vm.tags = res.data
    vm.tagSelect.options = _.map(res.data, function(d, i) {
      return {id: d.obj.revision, name: d.name}
    })

    // Sets 'compare to' version from the state if available
    // Sets the first tag from the list otherwise
    var found = _.findWhere(vm.tagSelect.options, {name: state.to})
    vm.tagSelect.selected = found
      ? found
      : _.first(vm.tagSelect.options)
  })

  $q.all([whenQueryRevisions, whenQueryTags]).then(function() {
    // Load grid data once revisions and tags loaded
    vm.updateData()
  })

  vm.updateData = function() {
    var revision = vm.revisionSelect.selected
    var baselineTag = vm.tagSelect.selected.name

    // Update permalink
    stateUtil.applyState(state, {
      from: revision,
      to: baselineTag,
    })

    // Set loading flag to display spinner
    vm.isLoading = true
    // Display no data while loading is in progress
    vm.gridOptions.data = []

    function isPatchId(revision) {
      return revision.length == EVG.PATCH_ID_LEN
    }

    // Depending on is revision patch id or version id
    // Different steps should be performed
    // Patch id requires additional step
    var chain = isPatchId(revision)
      ? ApiV2.getPatchById(revision).then(
          function(res) { return res.data.git_hash }, // Adaptor fn
          function(err) { // Hanle error
            console.error('Patch not found');
            return $q.reject()
          })
      : $q.resolve(revision)

    chain
      // Get version by revision
      .then(function(revision) {
        return ApiV1.getVersionByRevision(projectId, revision)
      })
      // Extract version object
      .then(function(res) {
        return res.data
      })
      // Load perf data
      .then(function(version) {
        return PerfDiscoveryDataService.getData(version, baselineTag)
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
