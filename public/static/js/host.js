mciModule.controller('HostCtrl', function($scope, $window) {
  $scope.userTz = $window.userTz;
  $scope.host = $window.host
  $scope.events = $window.events.reverse()
  if ($scope.host.host_type !== "static") {
      var uptime = moment().diff($scope.host.creation_time, 'seconds');
      $scope.host.uptime = moment.duration(uptime, 'seconds').humanize();
    } else {
      $scope.host.uptime = "N/A";
  }
});
