mciModule.factory('notificationService', function($rootScope) {
  var notifier = {};
  notifier.notifications = {};
  notifier.pushNotification = function(msg, destination, type) {
    // default is 'error'
    var type = type || "error"
    if (!this.notifications[destination] || this.notifications[destination].length === 0) {
        this.notifications[destination] = [];
    }
    if (_.pluck(this.notifications[destination], "msg").indexOf(msg) == -1) {
      this.notifications[destination].push({msg:msg, type:type});
      this.broadcastNotification();
    }
  };

  notifier.broadcastNotification = function() {
    $rootScope.$broadcast('broadcastNotification');
  };

  notifier.clear = function(destination) {
    notifier.notifications[destination] = [];
  };
  return notifier;
});

mciModule.controller('NotifyBoxCtrl', ['$scope','notificationService',function($scope, notificationService) {
  $scope.$on('broadcastNotification', function() {
    if (notificationService.notifications[$scope.destination]){
    $('#'+$scope.destination+'.alert.error-flash').show();
      $scope.notifications = notificationService.notifications[$scope.destination];
    }
  });
  $scope.close = function($event) {
    notificationService.clear($scope.destination);
    $($event.currentTarget).parents('#'+$scope.destination).hide();
  }
  $scope.notificationsForDest = function(destination){
    return notificationService.notifications[destination] || [];
  }
  $scope.notificationBoxClass = function(notification){
    // given a notification, look at its type and determine which CSS class 
    // should be used for the box to contain it when displayed
    var classes = {
        "success":"alert-success",
        "info":"alert-info",
        "warning":"alert-warning",
        "error":"alert-danger",
    };
    if(!notification['type'] || !(notification['type'] in classes)){
        return classes['error'];
    }
    return classes[notification['type']];
  }
}]);

mciModule.directive('notifyBox', function() {
  return {
    restrict: 'E',
    replace: true,
    templateUrl: '/static/partials/notifybox.html'
  };
});
