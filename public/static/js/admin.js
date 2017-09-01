mciModule.controller('AdminSettingsController', ['$scope','$window', 'mciAdminRestService', 'notificationService', function($scope, $window, mciAdminRestService, notificationService) {
  $scope.load = function() {
    $scope.Settings = {};
    $scope.getSettings();
  }

  $scope.getSettings = function() {
    var successHandler = function(resp) {
      $scope.Settings = resp;
    }
    var errorHandler = function(jqXHR, status, errorThrown) {
      notificationService.pushNotification("Error loading settings: " + jqXHR.error, "errorHeader");
    }
    mciAdminRestService.getSettings({ success: successHandler, error: errorHandler });
  }

  $scope.saveSettings = function() {
    var successHandler = function(resp) {
      window.location.href = "/admin";
    }
    var errorHandler = function(jqXHR, status, errorThrown) {
      notificationService.pushNotification("Error saving settings: " + jqXHR.error, "errorHeader");
    }
    mciAdminRestService.saveSettings($scope.Settings, { success: successHandler, error: errorHandler });
  }

  $scope.load()
}]);
