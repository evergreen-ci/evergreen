mciModule.controller('TaskQueuesCtrl',
  ['$scope', '$window', '$location', '$timeout', '$anchorScroll', 'mciTaskStatisticsRestService',
  function($scope, $window, $location, $timeout, $anchorScroll, taskStatisticsRestService) {

  $scope.taskQueues = $window.taskQueues;
  $scope.loading = true;
  $anchorScroll.yOffset = 60;

  $scope.distros = $window.distros.sort();

  $scope.queues = {};
  _.each($scope.taskQueues, function(queue) {
    $scope.queues[queue.distro] = queue.queue;
  });

  $scope.activeElement = $location.hash();
  $anchorScroll();

  $scope.setActiveElement = function(distro) {

    $scope.activeElement = distro;
    var old = $location.hash();
    $location.hash(distro);
    $anchorScroll();
    $location.hash(old);
  };

  $scope.isPatch = function(queueItem){
    return queueItem.requester != 'gitter_request';
  }

  $scope.sumEstimatedDuration = function(distro) {
    return _.reduce($scope.queues[distro], function(sum, queueItem){
      return sum + queueItem.exp_dur;
    }, 0)
  }

  $scope.getLength = function(distro){
    return $scope.queues[distro].length;
  }


}]);


