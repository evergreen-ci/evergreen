mciModule.controller('LoginCtrl', function($scope, $window, $location, mciLoginRestService) {

  $scope.credentials = {};
  $scope.errorMessage = '';
  $scope.redirect = $location.search().redirect;
  $scope.authenticate = function() {
    mciLoginRestService.authenticate(
        $scope.credentials.username,
        $scope.credentials.password,
        {},
        {
          success: function(resp) {
            redirect = $scope.redirect || "/";
            $window.location.href = decodeURIComponent(redirect);
          },
          error: function(resp) {
            $scope.errorMessage = resp.data.error;
          }
        }
    );
  };
});




mciModule.controller('LoginModalCtrl', function($scope, $window, mciLoginRestService, $controller) {

  // Inherit from LoginCtrl
  $controller('LoginCtrl', {$scope: $scope});

  $scope.openLoginModal = function() {
    if ($window.redirect) {
      $window.location.href = "/login/redirect";
    } else {
    var modal = $('#login-modal').modal('show');
    $('#username-modal').focus();
    }
  };
});

mciModule.directive('loginModal', function() {
  return {
    restrict: 'E',
    replace: true,
    templateUrl: '/static/partials/login_modal.html'
  };
});
