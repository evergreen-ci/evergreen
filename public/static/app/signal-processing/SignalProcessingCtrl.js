mciModule.controller('SignalProcessingCtrl', function(
  $window, Stitch, STITCH_CONFIG
) {
  var vm = this;
  var projectId = $window.project

  vm.isLoading = true
  Stitch.use(STITCH_CONFIG.PERF).query(function(db) {
    return db
      .db(STITCH_CONFIG.PERF.DB_PERF)
      .collection(STITCH_CONFIG.PERF.COLL_CHANGE_POINTS)
      .find({})
      .limit(100)
      .execute()
  }).then(function(docs) {
    vm.gridOptions.data = docs
  }, function(err) {
    console.error(err)
  }).finally(function() {
    vm.isLoading = false
  })

  vm.gridOptions = {
    enableFiltering: true,
    enableGridMenu: true,
    columnDefs: [
      {
        name: 'Project',
        field: 'project',
      },
      {
        name: 'Variant',
        field: 'variant',
      },
      {
        name: 'Task',
        field: 'task',
      },
      {
        name: 'Test',
        field: 'test',
      },
      {
        name: 'Revision',
        field: 'revision',
      },
      {
        name: 'Value',
        field: 'value',
        cellFilter: 'number:2',
      },
      {
        name: 'Value to Avg',
        field: 'value_to_avg',
        cellFilter: 'number:2',
      },
    ]
  }
})
