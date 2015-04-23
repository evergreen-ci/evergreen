mciModule.controller('LoginCtrl', function($scope, $window, mciLoginRestService) {

  $scope.credentials = {};
  $scope.errorMessage = '';

  $scope.authenticate = function() {
    mciLoginRestService.authenticate(
        $scope.credentials.username, 
        $scope.credentials.password,
        {},
        {
          success: function(data) {
            $window.location.href = decodeURIComponent(data.redirect);
          },
          error: function(jqXHR, status, errorThrown) {
            $scope.errorMessage = jqXHR;
          }
        }
    );
  };
});

mciModule.controller('LoginModalCtrl', function($scope, $window, mciLoginRestService, $controller) {

  // Inherit from LoginCtrl
  $controller('LoginCtrl', {$scope: $scope});

  $scope.openLoginModal = function() {
    var modal = $('#login-modal').modal('show');
    $('#username-modal').focus();
  };
});

mciModule.directive('loginModal', function() {
  return {
    restrict: 'E',
    replace: true,
    templateUrl: '/static/partials/login_modal.html'
  };
});
