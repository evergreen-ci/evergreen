mciModule.controller('HostCtrl', function($scope, $window) {
  $scope.userTz = $window.userTz;
  $scope.host = $window.host;
  $scope.running_task = $window.runningTask;
  $scope.events = $window.events.reverse();

  if ($scope.host.host_type !== "static") {
      var uptime = moment().diff($scope.host.creation_time, 'seconds');
      $scope.host.uptime = moment.duration(uptime, 'seconds').humanize();
    } else {
      $scope.host.uptime = "N/A";
  }

  // Determining the start and elapsed time should be done the same way as in hosts.js
  if ($scope.running_task && $scope.running_task.id) {
      var dispatchTimeDiffedPrefix = "";

      // In case the task is dispatched but not yet marked as
      // started, use the task's 'dispatch_time' in lieu of 'start_time'
      var epochTime = moment("Jan 1, 1970");
      var startTime = moment($scope.running_task.start_time);

      // 'start_time' is set to epochTime by default. We use
      // the <= comparison to allow for conversion imprecision
      if (startTime <= epochTime) {
        startTime = $scope.running_task.dispatch_time;
        dispatchTimeDiffedPrefix = "*";
      }

      var elapsedTime = moment().diff(startTime, 'seconds');
      $scope.host.start_time = startTime;
      $scope.host.elapsed = dispatchTimeDiffedPrefix + moment.duration(elapsedTime, 'seconds').humanize();
  } else {
      $scope.host.start_time = "N/A";
      $scope.host.elapsed = "N/A";
  }

  $scope.getStatusLabel = function(host) {
      if (host) {
        switch (host.status) {
        case 'running':
          return 'host-running';
        case 'provisioning':
        case 'starting':
          return 'host-starting';
        case 'decommissioned':
        case 'unreachable':
        case 'quarantined':
        case 'provision failed':
          return 'host-unreachable';
        case 'terminated':
          return 'host-terminated';
        default:
          return 'host-unreachable';
        }
      }
    }
});
