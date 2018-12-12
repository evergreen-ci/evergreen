mciModule.controller('PerfBBFailuresCtrl', function(
  ApiV2, $scope
) {
  // Perf Failures View-Model
  const vm = this
  const project = window.project

  vm.state = {
    lookBackDays: 14,
    status: ['failed'],
  }

  vm.lookBackDays = vm.state.lookBackDays

  vm.applyFiltering = function() {
    // Say the form the state is 'pristine'
    $scope.form.$setPristine()
    // Update controller state
    vm.state.lookBackDays = +vm.lookBackDays 
    loadData()
  }

  function loadData() {
    vm.isLoading = true
    ApiV2.getProjectTasks(project, {
      'started_after': moment().subtract({days: vm.state.lookBackDays}).format(),
      status: vm.state.status,
    }).then(function(res) {
      vm.gridOptions.data = res.data
      console.log(res.data)
    }).finally(function() {
      vm.isLoading = false
    })
  }

  loadData()

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    columnDefs: [{
      name: 'Create Time',
      field: 'create_time',
    }, {
      name: 'Task',
      field: 'display_name',
      cellTemplate: 'ui-grid-link',
      _link: function(row, col) {
        return '/task/' + row.entity.task_id
      }
    }, {
      name: 'Variant',
      field: 'build_variant',
      cellTemplate: 'ui-grid-link',
      _link: function(row, col) {
        return '/build/' + row.entity.build_id
      }
    }, {
      name: 'Fail Type',
      field: 'status_details.type',
    }, {
      name: 'Timed Out',
      field: 'status_details.timed_out',
    }, {
      name: 'Status',
      field: 'status',
      visible: false,
    }],
  }
})
