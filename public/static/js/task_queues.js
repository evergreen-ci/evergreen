mciModule.controller('TaskQueuesCtrl',
  ['$scope', '$window', '$location', '$timeout', '$anchorScroll', 'mciTaskStatisticsRestService',
  function($scope, $window, $location, $timeout, $anchorScroll, taskStatisticsRestService) {

  $scope.taskQueues = $window.taskQueues;
  $scope.loading = true;
  $scope.distros = [];

  $anchorScroll.yOffset = 60;

  _.each($scope.taskQueues, function(queue) {
    $scope.distros.push(queue.distro);
  })
  $scope.distros = $scope.distros.sort();

  $scope.queues = {};
  _.each($scope.taskQueues, function(queue) {
    $scope.queues[queue.distro] = queue.queue;
  });

  $scope.activeElement = $location.hash();

  $scope.setActiveElement = function(distro) {
    $scope.activeElement = distro;
    $location.hash(distro);
    $anchorScroll();
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

  $anchorScroll();

}]);


