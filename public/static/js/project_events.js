mciModule.controller('ProjectEventsController', ['$scope','$window', 'mciProjectRestService', 'eventsService', function($scope, $window, mciProjectRestService, eventsService) {
  $scope.userTz = $window.userTz;
  $scope.projectId = $window.projectId;
  $scope.nextTs = "";
  $scope.Events = [];

  $(window).scroll(function() {
      if ($(window).scrollTop() + $(window).height() == $(document).height()) {
        if ($scope.nextTs !== "") {
          eventsService.getMoreEvents($scope, mciProjectRestService, $scope.projectId);
        }
      }
    });

  // Get an initial batch of events
  eventsService.getMoreEvents($scope, mciProjectRestService, $scope.projectId);
}]);
