mciModule.controller('PerformanceDiscoveryCtrl', function(
  $q, $scope, $window, ApiTaskdata, ApiV1, PERF_DISCOVERY,
  PerfDiscoveryService
) {
  var vm = this;

  vm.revisionSelect = {
    options: [],
    selected: null,
  }

  vm.tagSelect = {
    options: [],
    selected: null,
  }

  var projectId = $window.project

  var whenQueryRevisions = ApiV1.getProjectVersions(projectId).then(function(res) {
    vm.versions = res.data.versions
    vm.revisionSelect.options = _.map(res.data.versions, function(d) {
      return {id: d.revision, name: d.revision}
    })
    vm.revisionSelect.selected = _.first(vm.revisionSelect.options)
  })

  var whenQueryTags = ApiTaskdata.getProjectTags(projectId).then(function(res){
    vm.tags = res.data
    vm.tagSelect.options = _.map(res.data, function(d, i) {
      return {id: d.obj.revision, name: d._id}
    })
    vm.tagSelect.selected = _.first(vm.tagSelect.options)
  })

  $q.all([whenQueryRevisions, whenQueryTags]).then(function() {
    // Load grid data once revisions and tags loaded
    vm.updateData()
  })

  vm.updateData = function() {
    var revision = vm.revisionSelect.selected.id;
    var baselineRev = vm.tagSelect.selected.id;

    version = _.findWhere(vm.versions, {
      revision: revision
    })

    var buildTaskData = _.reduce(version.builds, function(items, build) {
      var taskItems = _.map(build.tasks, function(v, k) {
        return {
          build: build.name,
          task: k,
          task_id: v.task_id,
        }
      })
      return items.concat(taskItems)
    }, [])

    var ctx = PerfDiscoveryService.extractTasks(version)

    PerfDiscoveryService.getRows(
      PerfDiscoveryService.processData(
        PerfDiscoveryService.queryData(
          ctx, baselineRev
        )
      )
    ).then(function(res) {
      vm.gridOptions.data = res
    })
  }

  vm.gridOptions = {
    //flatEntityAccess: true,
    minRowsToShow: 18,
    enableFiltering: true,
    columnDefs: [{
        name: 'Link',
        field: 'link',
        enableFiltering: true,
      }, {
        name: 'Build',
        field: 'build',
        enableFiltering: true,
      }, {
        name: 'Storage Engine',
        field: 'storageEngine',
        enableFiltering: true,
      }, {
        name: 'Task',
        field: 'task',
        enableFiltering: true,
      }, {
        name: 'Test',
        field: 'test',
        enableFiltering: true,
      }, {
        name: 'Threads',
        field: 'threads',
        enableFiltering: true,
      }, {
        name: 'Ratio',
        field: 'ratio',
        cellFilter: 'number:2',
      }, {
        name: 'Trend',
        field: 'trendData',
        enableSorting: false,
        width: PERF_DISCOVERY.TREND_COL_WIDTH,
      }, {
        name: 'Avg and Self',
        field: 'avgVsSelf',
        enableSorting: false,
      }, {
        name: 'ops/sec',
        field: 'speed',
        cellFilter: 'number:2',
      }, {
        name: 'Baseline',
        field: 'baseSpeed',
        cellFilter: 'number:2',
      },
    ]
  }
})
