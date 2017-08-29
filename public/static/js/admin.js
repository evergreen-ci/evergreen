mciModule.controller('AdminSettingsController', function($scope, $window, $location, $filter) {
  $scope.isSuperUser = $window.isSuperUser;
});
