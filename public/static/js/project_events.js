mciModule.controller('ProjectEventsController', ['$scope','$window', 'mciProjectRestService', 'eventsService', function($scope, $window, mciProjectRestService, eventsService) {
  $scope.userTz = $window.userTz;
  $scope.projectId = $window.projectId;
  $scope.Events = [];

  eventsService.setupWindow($scope, mciProjectRestService);

  // Get an initial batch of events
  eventsService.getMoreEvents($scope, mciProjectRestService, "", $scope.projectId);
}]);
