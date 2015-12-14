mciModule.controller('HostsCtrl', function($scope, $filter, $window, $location) {
  $scope.selectedHeader = {};
  $scope.headerFields = [{
      name: 'Id',
      by: 'host',
      order: false,
    },
    {
      name: 'Distro',
      by: 'distro._id',
      order: false,
    },
    {
      name: 'Status',
      by: 'status',
      order: false,
    },
    {
      name: 'Current task',
      by: 'task',
      order: false,
    },
    {
      name: 'Elapsed',
      by: 'start_time',
      order: false,
    },
    {
      name: 'Uptime',
      by: 'creation_time',
      order: false,
    },
    {
      name: 'Owner',
      by: 'started_by',
      order: false,
    },
  ]

  $scope.toggleIncludeSpawnedHosts = function(includeSpawnedHosts) {
      $window.location.href = "/hosts?includeSpawnedHosts=" + includeSpawnedHosts;
  };

  $scope.selectedClass = function(headerField) {
    var newIcon = 'fa-sort';
    if (headerField.name == $scope.selectedHeader.name) {
      newIcon = 'fa-sort-up';
      if ($scope.selectedHeader.order) {
        newIcon = 'fa-sort-down';
      }
    }
    return newIcon;
  }

  $scope.setSelectedHeader = function(headerField) {
    if ($scope.selectedHeader.name == headerField.name) {
      $scope.selectedHeader.order = !$scope.selectedHeader.order;
    } else {
      $scope.selectedHeader = headerField;
      $scope.selectedHeader.order = false;
    }
  };

  // format displayed uptime and task elapsed time
  var allHosts = $window.hosts.Hosts;
  $scope.hosts = [];
  var filterOpts = $location.path().split('/');
  $scope.filter = {
    hosts : filterOpts[2] || ''
  };
  $scope.selectAll = false;
  $scope.filteredHosts = $scope.hosts;

  $scope.$watch('filter.hosts', function() {
    $location.path('filter/' + $scope.filter.hosts);
  });

  _.forEach(allHosts, function(hostObj) {
    var host = {};
    var hostDoc = hostObj.Host;
    host.host_type = hostDoc.host_type;
    host.distro = hostDoc.distro;
    host.status = hostDoc.status;
    host.id = hostDoc.id;
    host.host = hostDoc.host;
    host.creation_time = hostDoc.creation_time;
    host.started_by = hostDoc.started_by;
    host.task = "";
    host.running_task = hostObj.RunningTask;

    if (hostDoc.host_type !== "static") {
      var uptime = moment().diff(hostDoc.creation_time, 'seconds');
      host.uptime = moment.duration(uptime, 'seconds').humanize();
    } else {
      host.creation_time = "N/A";
      host.uptime = "N/A";
    }
    if (hostObj.RunningTask) {
      host.start_time = hostObj.RunningTask.start_time
      var dispatchTimeDiffedPrefix = "";
      // in case the task is dispatched but not yet marked as
      // started, use the task's 'dispatch_time' in lieu of
      // 'start_time'
      var epochTime = moment("Jan 1, 1970");
      var startTime = moment(hostObj.RunningTask.start_time);
      // 'start_time' is set to epochTime by default we use
      // the <= comparison to allow for conversion imprecision
      if (startTime <= epochTime) {
        host.start_time = hostObj.RunningTask.dispatch_time;
        startTime = hostObj.RunningTask.dispatch_time;
        dispatchTimeDiffedPrefix = "*";
      }
      var elapsedTime = moment().diff(startTime, 'seconds');
      host.task = hostObj.RunningTask.display_name;
      host.elapsed = dispatchTimeDiffedPrefix + moment.duration(elapsedTime, 'seconds').humanize();
    } else {
      host.start_time = "N/A";
      host.elapsed = "N/A";
    }
    $scope.hosts.push(host);
  });

  $scope.selectedHosts = function() {
    return $filter('filter')($scope.hosts, {checked: true});
  };

  $scope.toggleHostCheck = function(host) {
    host.checked = !host.checked;
  };

  $scope.toggleSelectAll = function() {
    $scope.selectAll = !$scope.selectAll;
    $scope.setCheckBoxes($scope.selectAll);
  };
    
  $scope.clearSelectAll = function() {
    $scope.selectAll = false;
    $scope.setCheckBoxes($scope.selectAll);
  };

  $scope.setCheckBoxes = function(val) {
    for (idx in $scope.filteredHosts) {
        $scope.filteredHosts[idx].checked = val;
    }
  };

  $scope.$watch('hosts', function(hosts) {
    $scope.hostCount = 0;
    _.forEach(hosts, function(host) {
      if(host.checked){
        $scope.hostCount += 1;
      }
    });
  }, true);

});
