mciModule.controller('PerfBBWarningsCtrl', function(
  ApiV2, $scope
) {
  // Perf Warnings View-Model
  var vm = this
  var project = window.project

  vm.state = {
    lookBackDays: 14,
    status: ['failed'],
  }

  vm.lookBackDays = vm.state.lookBackDays

  vm.applyFiltering = function() {
    // ay the form the state is 'pristine'
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
    }).finally(function() {
      vm.isLoading = false
    })
  }

  loadData()

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    columnDefs: [{
      name: 'Name',
      field: 'display_name',
    }, {
      name: 'Variant',
      field: 'build_variant',
    }, {
      name: 'Created',
      field: 'create_time',
    }, {
      name: 'Status',
      field: 'status',
    }, {
      name: 'Fail Type',
      field: 'status_details.type',
    }, {
      name: 'Timed Out',
      field: 'status_details.timed_out',
    }],
  }
})
