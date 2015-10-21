mciModule.factory('errorPasserService', function($rootScope) {
  var errorPasser = {};
  errorPasser.errors = {};
  errorPasser.pushError = function(errorMsg, destination) {
    if (!this.errors[destination] || this.errors[destination].length === 0) {
    this.errors[destination] = [];
    }
    if (this.errors[destination].indexOf(errorMsg) == -1) {
      this.errors[destination].push(errorMsg);
      this.broadcastError();
    }
  };

  errorPasser.broadcastError = function() {
    $rootScope.$broadcast('broadcastError');
  };

  errorPasser.clearErrors = function(destination) {
    errorPasser.errors[destination] = [];
  };
  return errorPasser;
});

mciModule.controller('ErrorCtrl', ['$scope','errorPasserService',function($scope, errorPasserService) {
  $scope.$on('broadcastError', function() {
    if (errorPasserService.errors[$scope.destination]){
    $('#'+$scope.destination+'.alert.error-flash').show();
      $scope.errorMessages = errorPasserService.errors[$scope.destination];
    }
  });
  $scope.close = function($event) {
    errorPasserService.clearErrors($scope.destination);
    $($event.currentTarget).parents('#'+$scope.destination).hide();
  }
}]);

mciModule.directive('errorFlash', function() {
  return {
    restrict: 'E',
    replace: true,
    templateUrl: '/static/partials/error_flash.html'
  };
});
