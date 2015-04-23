mciModule.controller('TaskQueuesCtrl',
  ['$scope', '$window', '$location', 'mciTaskStatisticsRestService',
  function($scope, $window, $location, taskStatisticsRestService) {

  $scope.taskQueues = $window.taskQueues;
  $scope.hostStats = $window.hostStats;

  $scope.taskStats = []

  // these objects store the options for the dropdown on
  // task timing statistic aggregations. The controller automatically
  // fills in the <select> tags in the ui
  $scope.statOptions = {
    fieldOptions: [
      {display: "Start \u21E2 Finish",
        val:["start_time", "finish_time"]},
      {display: "Scheduled \u21E2 Start",
        val:["scheduled_time", "start_time"]},
      {display: "Scheduled \u21E2 Dispatched",
        val:["scheduled_time", "dispatch_time"]},
      {display: "Created \u21E2 Scheduled",
        val:["create_time", "scheduled_time"]},
    ],
    groupOptions: [
      {display: "Distro", val:"distro"},
      {display: "Task Name", val:"display_name"},
      {display: "Build Variant", val:"build_variant"},
    ],
    timeOptions: [
      {display: "Today", val:"1"},
      {display: "One Week", val:"7"},
      {display: "One Month", val:"30"},
      {display: "All Time", val:"-1"},
    ]
  }
  $scope.fields = $scope.statOptions.fieldOptions[0];
  $scope.group = $scope.statOptions.groupOptions[0];
  $scope.cutoff = $scope.statOptions.timeOptions[0];

  $scope.loading = true;

  // function for making an ajax call to the task timing aggreation
  // ui endpoint. Updates UI with simple loading text while waiting.
  $scope.getTimeStats = function getTimeStats() {
    $scope.loading = true
    taskStatisticsRestService.getTimeStatistics(
      $scope.fields.val[0], $scope.fields.val[1],
      $scope.group.val, $scope.cutoff.val, {
        success: function(data, status) {
          if (!data || data == "null") {
            $scope.taskStats = [];
          } else {
            $scope.taskStats = data;
          }
          $scope.loading = false;
        },
        error: function(jqXHR, status, errorThrown) {
          $scope.loading = false;
          console.log('Error getting statistics ' + jqXHR);
        }
      });
  };

  // watch dropdowns for a change.
  // TODO: If we update angular, we can
  // merge this into one $watchCollection command
  $scope.$watch('fields', function(newValue, oldValue) {
    if (newValue != oldValue)
      $scope.getTimeStats();
  });
  $scope.$watch('group', function(newValue, oldValue) {
    if (newValue != oldValue)
      $scope.getTimeStats();
  });
  $scope.$watch('cutoff', function(newValue, oldValue) {
    if (newValue != oldValue)
      $scope.getTimeStats();
  });

  $scope.distros = [];
  _.each($scope.taskQueues, function(queue) {
    $scope.distros.push(queue.distro);
  })
  $scope.distros = $scope.distros.sort();

  $scope.queues = {};
  _.each($scope.taskQueues, function(queue) {
    $scope.queues[queue.distro] = queue.queue;
  });

  $scope.activeDistro = $location.hash();

  $scope.setActiveDistro = function(distro) {
    $scope.activeDistro = distro;
    $location.hash(distro);
    $('body').animate({scrollTop: $('#'+distro).offset().top-60}, 'fast');
  };

  $scope.requesterColumn = function(queueItem) {
    if (queueItem.requester === 'gitter_request') {
      return queueItem.project + ' (' +
        '<span style="font-family: monospace">' +
        queueItem.gitspec.substring(0, 8) +
        '</span>)';
    }
    return 'Patch by ' + queueItem.user;
  };

}]);


