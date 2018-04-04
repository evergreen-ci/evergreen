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

// XXX: If you are changing the key validation here, you must also update
// (h *keysPostHandler) validatePublicKey in rest/route/keys_routes.go to
// match
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

// XXX: if modifying any of the password validation logic, you changes must
// also be ported into spawn/spawn.go

// the regexes below is used to ensure that a string meets the windows
// password complexity requirements as described at:
// http://technet.microsoft.com/en-us/library/cc786468(v=ws.10).aspx
var passwordRegexps =
[
        // XXX: This is technically wrong, however even ES6 does
        // not support unicode character classes (\p{...}). In spite of this,
        // this should work with the vast majority of users. The server
        // will also back this up with the correct validation
	/[a-z]/, // should be \p{Lowercase_Letter}
	/[A-Z]/, // should be \p{Uppercase_Letter}
	/[0-9]/,
	/[~!@#$%^&*_\-+=|\\\(\){}\[\]:;"'<>,.?\/`]/,
	/[^A-Za-z~!@#$%^&*_\-+=|\\\(\){}\[\]:;"'<>,.?\/`]/, // should be \p{Lo}, for characters without upper/lower case variants
]

directives.spawn.directive('complexity', function() {
  return {
    require: 'ngModel',
    link: function(scope, elm, attrs, ctrl) {
      ctrl.$parsers.unshift(function(viewValue) {
        // validate the password requirements
        var glyphs = Array.from(viewValue)
        if(glyphs.length < 6 || glyphs.length > 255) {
          ctrl.$setValidity('complexity', false);
          return viewValue;
        }

        var matches = 0
        for(var i = 0; i < passwordRegexps.length; i++) {
            if(passwordRegexps[i].test(viewValue)) {
                matches++
            }
        }
        ctrl.$setValidity('complexity', matches >= 3);
        return viewValue;
      });
    }
  };
});
