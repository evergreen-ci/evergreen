mciModule.controller('PerformanceDiscoveryCtrl', function($scope) {
  var vm = this;

  vm.revisionSelect = {
    options: [
      {id: 1, name: '4ef5fx',},
      {id: 2, name: 'v52fe3',},
      {id: 3, name: 'd43ap0',},
    ],
  }

  vm.revisionSelect.selected = vm.revisionSelect.options[0]

  vm.tagSelect = {
    options: [
      {id: 1, name: 'v3.6.1',},
      {id: 2, name: 'v3.6.0',},
      {id: 3, name: 'v3.5.2',},
    ],
  }

  vm.tagSelect.selected = vm.tagSelect.options[0]

  vm.gridOptions = {
    enableSorting: true,
    coumnDefs: [{
        name: 'Link',
        field: 'link',
        enableFiltering: true,
      }, {
        name: 'Build',
        field: 'build',
        enableFiltering: true,
      }, {
        name: 'Storage Engine',
        field: 'storage_engine',
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
      }, {
        name: 'Trend',
        field: 'trend',
        enableSorting: false,
      }, {
        name: 'Avg and Self',
        field: 'avg_and_self',
        enableSorting: false,
      }, {
        name: 'ops/sec',
        field: 'ops_per_sec',
      }, {
        name: 'Baseline',
        field: 'baseline',
      },
    ],
    data: [{
      link: 'some link',
      build: 'build',
      storage_engine: 'st eng',
      task: 'some data',
      test: 'some data',
      threads: '16',
      trend: '',
      avg_and_self: '',
      ops_per_sec: 15302,
      baseline: 23597,
    }, {
      link: 'some link 2',
      build: 'build 2',
      storage_engine: 'st eng 2',
      task: 'some data',
      test: 'some data',
      threads: '32',
      trend: '',
      avg_and_self: '',
      ops_per_sec: 15303,
      baseline: 23591,
    }]
  }
})
