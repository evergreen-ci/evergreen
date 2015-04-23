var directives = directives || {};

directives.events = angular.module('directives.events', []);

directives.events.directive('mciBlur', function() {
  return function(scope, element, attrs) {
    $(element).on("blur", function() {
      // Timeout is necessary, because of a common use case where you want
      // an autocomplete to disappear when the input loses focus. Clicking on
      // an autocomplete element counts as losing focus, and then the click
      // doesn't register because the element is queued up to disappear.
      setTimeout(function() {
        scope.$eval(attrs.mciBlur);
        scope.$apply();
      }, 250);
    });
  };
});

directives.events.directive('mciFocus', function() {
  return function(scope, element, attrs) {
    $(element).on("focus", function() {
      scope.$eval(attrs.mciFocus);
      scope.$apply();
    });
  };
});

// Port of Angular 1.2.x ngMouseenter, ngMouseleave
directives.events.directive('mciMouseenter', function($parse) {
  return function(scope, element, attrs) {
    $(element).on("mouseover", function() {
      scope.$eval(attrs.mciMouseenter);
      scope.$apply();
    });
  };
});

directives.events.directive('mciMouseleave', function($parse) {
  return function(scope, element, attrs) {
    $(element).on("mouseout", function() {
      scope.$eval(attrs.mciMouseleave);
      scope.$apply();
    });
  };
});

(function() {
  var keyUpHandlerGenerator = function(attr, keyCode) {
    return function(scope, element, attrs) {
      element.bind('keyup', function(evt) {
        if (evt.which === keyCode) {
          scope.$eval(attrs[attr]);
          scope.$apply();
        }
      });
    };
  };

  directives.events.directive('mciKeyReturn', function() {
    return keyUpHandlerGenerator('mciKeyReturn', 13);
  });
})();
