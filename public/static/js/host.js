mciModule.controller('HostCtrl', function($scope, $window) {
  $scope.userTz = $window.userTz;
  $scope.host = $window.host;
  $scope.running_task = $window.runningTask;
  $scope.events = $window.events.reverse();
  $scope.containers = $window.containers;
  $scope.readOnly = (!window.aclEnabled && !$window.isSuperUser) || ($window.aclEnabled && $window.permissions.distro_hosts < 20) 

  // Spawned by task aliases
  $scope.spawnedByTask = $scope.host.spawn_options.spawned_by_task
  $scope.taskId = $scope.host.started_by

  $scope.host.uptime = "N/A";
  if ($scope.host.host_type !== "static") {
      var uptime;
      if($scope.host.status == "terminated"){
        uptime = moment($scope.host.termination_time).diff($scope.host.creation_time, 'seconds');
      }else{
        uptime = moment().diff($scope.host.creation_time, 'seconds');
      }
      $scope.host.uptime = moment.duration(uptime, 'seconds').humanize();
  }

  // FIXME this might be inaccurate, because this will include local timezone
  //       we might want moment(0) for '1970-01-01T00:00:00Z'
  var epochTime = moment("Jan 1, 1970");

  var last_communication = moment($scope.host.last_communication);
  if (last_communication <= epochTime) {
	  $scope.host.last_communication = "N/A";
  } else {
    var last_communication_seconds = moment().diff($scope.host.last_communication, 'seconds');
    $scope.host.last_communication = moment.duration(last_communication_seconds, 'seconds').humanize() + ' ago';
  }

  // Determining the start and elapsed time should be done the same way as in hosts.js
  if ($scope.running_task && $scope.running_task.id) {
      var dispatchTimeDiffedPrefix = "";

      // In case the task is dispatched but not yet marked as
      // started, use the task's 'dispatch_time' in lieu of 'start_time'
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
