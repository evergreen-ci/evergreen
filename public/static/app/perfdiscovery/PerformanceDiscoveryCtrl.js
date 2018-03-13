mciModule.controller('PerformanceDiscoveryCtrl', function(
  $q, $scope, $window, ApiTaskdata, ApiV1, EvgUiGridUtil,
  PERF_DISCOVERY, PerfDiscoveryService, uiGridConstants
) {
  var vm = this;
  var gridUtil = EvgUiGridUtil

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
    // 40 is githash length; don't allow user type invalid githash
    if (query.length == 40 && opts.indexOf(query) == -1) {
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

    ApiV1.getVersionByRevision(projectId, revision).then(function(res) {
      var version = res.data
      PerfDiscoveryService.getData(
        version, baselineTag
      ).then(function(res) {
        vm.gridOptions.data = res

        // Apply options data to filter drop downs
        gridUtil.applyMultiselectOptions(
          res,
          ['build', 'storageEngine', 'task', 'threads'],
          vm.gridOptions
        )
      })
    })
  }

  vm.gridOptions = {
    minRowsToShow: 18,
    enableFiltering: true,
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
      }),
      gridUtil.multiselectColDefMixin({
        name: 'Storage Engine',
        field: 'storageEngine',
      }),
      gridUtil.multiselectColDefMixin({
        name: 'Task',
        field: 'task',
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
        name: 'Ratio, %',
        field: 'ratio',
        type: 'number',
        cellTemplate: '<perf-discovery-ratio ratio="COL_FIELD"/>',
        enableFiltering: false,
        sort: {
          direction: uiGridConstants.DESC,
        },
        width: 80,
      },
      {
        name: 'Trend',
        field: 'trendData',
        type: 'number',
        width: PERF_DISCOVERY.TREND_COL_WIDTH,
        enableSorting: false,
        enableFiltering: false,
      },
      {
        name: 'Avg and Self',
        field: 'avgVsSelf',
        type: 'number',
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
