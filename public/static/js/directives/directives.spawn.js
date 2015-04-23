var directives = directives || {};

directives.spawn = angular.module('directives.spawn', ['filters.common']);

directives.spawn.directive('userSpawnModal', function() {
  return {
    restrict: 'E',
    transclude: true,
    templateUrl: '/static/partials/user_spawn_modal.html'
  }
});

directives.spawn.directive('userHostOptions', function() {
  return {
    restrict: 'E',
    templateUrl: '/static/partials/user_host_options.html'
  }
});

directives.spawn.directive('userHostDetails', function() {
  return {
    restrict: 'E',
    templateUrl: '/static/partials/user_host_details.html'
  }
});

directives.spawn.directive('userHostTerminate', function() {
  return {
    restrict: 'E',
    templateUrl: '/static/partials/user_host_terminate.html'
  }
});

directives.spawn.directive('userHostUpdate', function() {
  return {
    restrict: 'E',
    templateUrl: '/static/partials/user_host_update.html'
  }
});

// Validation directives
directives.spawn.directive('keyNameUnique', function() {
  return {
    require: 'ngModel',
    link: function(scope, elm, attrs, ctrl) {
      ctrl.$parsers.unshift(function(viewValue) {
        if (viewValue === '') {
          ctrl.$setValidity('keyNameUnique', false);
          return viewValue;
        }
        // ensure the key name isn't taken
        for (var i = 0; i < scope.userKeys.length; i++) {
          if (scope.userKeys[i].name === viewValue) {
            ctrl.$setValidity('keyNameUnique', false);
            return viewValue;
          }
        }
        ctrl.$setValidity('keyNameUnique', true);
        return viewValue;
      });
    }
  };
});

directives.spawn.directive('userDataValid', function() {
  return {
    require: 'ngModel',
    link: function(scope, elm, attrs, ctrl) {
      ctrl.$parsers.unshift(function(viewValue) {
        if (scope.selectedDistro.userDataValidate == 'x-www-form-urlencoded') {
          var arr = viewValue.split('&');
          if (arr.length == 0) {
            ctrl.$setValidity('userDataValid', false);
            return viewValue;
          }
          for(var n = 0; n < arr.length; n++) {
            var item = arr[n].split("=");
            if (item.length != 2) {
                ctrl.$setValidity('userDataValid', false);
                return viewValue;
            }
          }
        } else if (scope.selectedDistro.userDataValidate == 'json') {
          try {
            JSON.parse(viewValue);
          } catch (e) {
            ctrl.$setValidity('userDataValid', false);
            return viewValue;
          }
        } else if (scope.selectedDistro.userDataValidate == 'yaml') {
          try {
            jsyaml.load(viewValue);
          } catch (e) {
            ctrl.$setValidity('userDataValid', false);
            return viewValue;
          }
        }
        ctrl.$setValidity('userDataValid', true);
        return viewValue;
      });
    }
  };
});

var RSA = 'ssh-rsa';
var DSS = 'ssh-dss';
var BASE64REGEX = new RegExp("^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{4})$");

directives.spawn.directive('keyBaseValid', function() {
  return {
    require: 'ngModel',
    link: function(scope, elm, attrs, ctrl) {
      ctrl.$parsers.unshift(function(viewValue) {
        // validate the public key
        var isRSA = viewValue.substring(0, RSA.length) === RSA;
        var isDSS = viewValue.substring(0, DSS.length) === DSS;
        if (!isRSA && !isDSS) {
          ctrl.$setValidity('keyBaseValid', false);
          return viewValue;
        }
        // do pseudo key validation
        var sections = viewValue.split(' ');
        if (sections.length < 2) {
          ctrl.$setValidity('keyBaseValid', false);
          return viewValue;
        }
        ctrl.$setValidity('keyBaseValid', BASE64REGEX.test(sections[1]));
        return viewValue;
      });
    }
  };
});

directives.spawn.directive('equals', function() {
  return {
    require: 'ngModel',
    link: function(scope, elem, attrs, ctrl) {
      // watch own value and re-validate on change
      scope.$watch(attrs.ctrl, function() {
        ensureEquals();
      });

      // observe the other value and re-validate on change
      attrs.$observe('equals', function (val) {
        ensureEquals();
      });

      var ensureEquals = function() {
        // values
        var password = ctrl.$viewValue;
        var cPassword = attrs.equals;
        // set validity
        scope.updateForm.uPassword.$setValidity("equals", password === cPassword);
        scope.updateForm.cPassword.$setValidity("equals", password === cPassword);
      };
    }
  }
});

// the regex below is used to ensure that a string meets the windows
// password complexity requirements as described at:
// http://technet.microsoft.com/en-us/library/cc786468(v=ws.10).aspx
var passwordRegex = /(?=^.{6,255}$)((?=.*\d)(?=.*[A-Z])(?=.*[a-z])|(?=.*\d)(?=.*[^A-Za-z0-9])(?=.*[a-z])|(?=.*[^A-Za-z0-9])(?=.*[A-Z])(?=.*[a-z])|(?=.*\d)(?=.*[A-Z])(?=.*[^A-Za-z0-9]))^.*/;

directives.spawn.directive('complexity', function() {
  return {
    require: 'ngModel',
    link: function(scope, elm, attrs, ctrl) {
      ctrl.$parsers.unshift(function(viewValue) {
        // validate the password requirements
        ctrl.$setValidity('complexity', passwordRegex.test(viewValue));
        return viewValue;
      });
    }
  };
});
