mciModule.controller('PerformanceDiscoveryCtrl', function(
  $scope, $q, ApiTaskdata, ApiV1, PerfDiscoveryService, PERF_DISCOVERY
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

  // TODO use globally defined project id
  var projectId = 'sys-perf'

  ApiV1.getProjectVersions(projectId).then(function(data) {
    vm.versions = data.data.versions
    vm.revisionSelect.options = _.map(data.data.versions, function(d, i) {
      return {id: i, name: d.revision}
    })
    vm.revisionSelect.selected = vm.revisionSelect.options[0]

    vm.tagSelect.options = _.map(data.data.versions, function(d, i) {
      return {id: i, name: d.revision}
    })
    // Choose the second item, if exists, or the first if no
    vm.tagSelect.selected = vm.tagSelect.options[
      _.min([1, vm.tagSelect.options.length - 1])
    ]
    vm.changeRevision(data.data.versions[0].revision)
  })

  vm.changeRevision = function(revision) {
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
          ctx, version, vm.versions[1]
        )
      )
    ).then(function(res) {
      vm.gridOptions.data = res
    })

  }

  vm.gridOptions = {
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
