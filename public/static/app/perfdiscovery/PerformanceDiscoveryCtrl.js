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
      return {id: d.obj.revision, name: d.name}
    })
    vm.tagSelect.selected = _.first(vm.tagSelect.options)
  })

  $q.all([whenQueryRevisions, whenQueryTags]).then(function() {
    // Load grid data once revisions and tags loaded
    vm.updateData()
  })

  function extractFilterOptions(items, idKey, nameKey) {
    return _.map(items, function(d) {
      return {id: d[idKey], name: d[nameKey]}
    })
  }

  function colByField(gridOptions) {
    return function(fieldName) {
      return _.findWhere(gridOptions.columnDefs, {
        field: fieldName
      })
    }
  }

  vm.updateData = function() {
    var revision = vm.revisionSelect.selected.id;
    var baselineRev = vm.tagSelect.selected.id;

    version = _.findWhere(vm.versions, {
      revision: revision
    })

    PerfDiscoveryService.getData(
      version, baselineRev
    ).then(function(res) {
      vm.gridOptions.data = res
      var col = colByField(vm.gridOptions)
      col('build').filter.options = _.unique(_.pluck(res, 'build'))
      col('storageEngine').filter.options = _.unique(_.pluck(res, 'storageEngine'))
      col('task').filter.options = _.unique(_.pluck(res, 'task'))
      col('threads').filter.options = _.unique(_.pluck(res, 'threads'))
    })
  }

  function conditionFn(term, val, row, col) {
    if (typeof(term) == 'undefined') return true
    if (term.length === 0) return true
    return _.contains(term, val)
  }

  vm.gridOptions = {
    //flatEntityAccess: true,
    minRowsToShow: 18,
    enableFiltering: true,
    columnDefs: [{
        name: 'Link',
        field: 'link',
        enableFiltering: false,
        width: 60,
      }, {
        name: 'Build',
        field: 'build',
        filterHeaderTemplate: 'evg-ui-select/header-filter',
        filter: {
          noTerm: true,
          rawTerm: true,
          condition: conditionFn,
          options: []
        },
      }, {
        name: 'Storage Engine',
        field: 'storageEngine',
        filterHeaderTemplate: 'evg-ui-select/header-filter',
        filter: {
          noTerm: true,
          condition: conditionFn,
          options: []
        },
      }, {
        name: 'Task',
        field: 'task',
        filterHeaderTemplate: 'evg-ui-select/header-filter',
        filter: {
          noTerm: true,
          condition: conditionFn,
          options: []
        },
      }, {
        name: 'Test',
        field: 'test',
      }, {
        name: 'Threads',
        field: 'threads',
        filterHeaderTemplate: 'evg-ui-select/header-filter',
        filter: {
          noTerm: true,
          condition: conditionFn,
          options: []
        },
        width: 130,
      }, {
        name: 'Ratio',
        field: 'ratio',
        cellFilter: 'number:2',
        enableFiltering: false,
        width: 70,
      }, {
        name: 'Trend',
        field: 'trendData',
        width: PERF_DISCOVERY.TREND_COL_WIDTH,
        enableSorting: false,
        enableFiltering: false,
      }, {
        name: 'Avg and Self',
        field: 'avgVsSelf',
        enableSorting: false,
        enableFiltering: false,
      }, {
        name: 'ops/sec',
        field: 'speed',
        cellFilter: 'number:2',
        enableFiltering: false,
        width: 100,
      }, {
        name: 'Baseline',
        field: 'baseSpeed',
        cellFilter: 'number:2',
        enableFiltering: false,
        width: 120,
      },
    ]
  }
})

mciModule.run(function($templateCache) {
  $templateCache.put('perf-discovery/multiselect-filter',
    '<ui-select multiple append-to-body="true" ng-model="vm.col.filters[0].term" ng-change="vm.triggerFiltering()">' +
      '<ui-select-match placeholder="Choose {{vm.field}}">{{$item}}</ui-select-match>' +
      '<ui-select-choices repeat="item in vm.filters">' +
        '{{item}}' +
      '</ui-select-choices>' +
    '</ui-select>'
  )

  $templateCache.put('evg-ui-select/header-filter',
    '<mselect field="col.field" filters="col.filters[0].options" grid="grid" col="col"></mselect>'
  )
})

mciModule.directive('mselect', function(uiGridConstants) { return {
  restrict: 'E',
  scope: {
    filters: '=',
    field: '=',
    grid: '=',
    col: '=',
  },
  bindToController: true,
  controllerAs: 'vm',
  templateUrl: 'perf-discovery/multiselect-filter',
  controller: function() {
    var vm = this

    vm.triggerFiltering = function() {
      vm.grid.notifyDataChange(uiGridConstants.dataChange.COLUMN)
    }

    vm.model = []//vm.col.filters[0].term

    return vm
  }
}})
