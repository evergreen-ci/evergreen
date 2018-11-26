mciModule.controller('AdminEventsController', ['$scope','$window', 'mciAdminRestService', 'notificationService', 'eventsService', function($scope, $window, mciAdminRestService, notificationService, eventsService) {
  $scope.userTz = $window.userTz;
  $scope.nextTs = "";
  $scope.Events = [];

  $(window).scroll(function() {
      if ($(window).scrollTop() + $(window).height() == $(document).height()) {
        if ($scope.nextTs !== "") {
          eventsService.getMoreEvents($scope, mciAdminRestService);
        }
      }
    });

  // Get an initial batch of events
  eventsService.getMoreEvents($scope, mciAdminRestService);

  $scope.revertEvent = function(guid) {
    var successHandler = function(resp) {
      window.location.href = "/admin/events";
    }
    var errorHandler = function(resp) {
      notificationService.pushNotification("Error reverting settings: " + resp.data.error, "errorHeader");
    }
    mciAdminRestService.revertEvent(guid, { success: successHandler, error: errorHandler });
  }
}]);
