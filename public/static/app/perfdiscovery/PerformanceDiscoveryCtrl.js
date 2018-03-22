mciModule.controller('PerformanceDiscoveryCtrl', function(
  $q, $scope, $window, ApiTaskdata, ApiV1, EvgUiGridUtil,
  PERF_DISCOVERY, PerfDiscoveryService, uiGridConstants
) {
  var vm = this;
  var gridUtil = EvgUiGridUtil
  var grid

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
    if (_.contains([24, 40], query.length) && opts.indexOf(query) == -1) {
      return opts.concat(query)
    }
    return opts
  }

  var whenQueryRevisions = ApiV1.getWaterfallVersionsRows(projectId).then(function(res) {
    vm.versions = _.map(
      _.where(
        res.data.versions, {rolled_up: false} // Filter versions with data
      ),
      function(d) { return {revision: d.revisions[0]} } // Transform
    )

    vm.revisionSelect.options = _.map(vm.versions, function(d) {
      return d.revision
    })
    vm.revisionSelect.selected = _.first(vm.revisionSelect.options)
  })

  var whenQueryTags = ApiTaskdata.getProjectTags(projectId).then(function(res){
    vm.tags = res.data
    vm.tagSelect.options = _.map(res.data, function(d, i) {
      return {id: d.obj.revision, name: d.name}
    })
    vm.tagSelect.selected = _.first(vm.tagSelect.options)
  })

  $q.all([whenQueryRevisions, whenQueryTags]).then(function() {
    // Load grid data once revisions and tags loaded
    vm.updateData()
  })

  vm.updateData = function() {
    var revision = vm.revisionSelect.selected
    var baselineTag = vm.tagSelect.selected.name

    // Set loading flag to display spinner
    vm.isLoading = true
    // Display no data while loading is in progress
    vm.gridOptions.data = []

    ApiV1.getVersionByRevision(projectId, revision).then(function(res) {
      var version = res.data
      PerfDiscoveryService.getData(version, baselineTag).then(function(res) {
        vm.gridOptions.data = res
        // Apply options data to filter drop downs
        gridUtil.applyMultiselectOptions(
          res,
          ['build', 'storageEngine', 'task', 'threads'],
          vm.gridOptions
        )
      }).finally(function() { vm.isLoading = false })
    })
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
    onRegisterApi: function(gridApi) {
      grid = gridApi.grid;
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
        sort: {
          direction: uiGridConstants.DESC,
        },
        width: 80,
      },
      {
        name: 'Average Ratio',
        field: 'avgRatio',
        type: 'number',
        cellTemplate: '<perf-discovery-ratio ratio="COL_FIELD"/>',
        enableFiltering: false,
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
