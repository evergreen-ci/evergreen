var directives = directives || {};

directives.svg = angular.module('directives.svg', []);

// mciCanvas defines an abstraction layer around the actual size of the SVG
// element. Since style overrides the 'height' and 'width' attributes, we can
// use the 'height' and 'width' attributes to specify the size of the logical
// canvas, and use the X() and Y() to map coordinates on the constant-size
// logical canvas to coordinates on the variably-sized SVG.
directives.svg.directive('mciCanvas', function() {
  return function(scope, element, attrs) {
    var projected = {
      height: attrs.height,
      width: attrs.width
    };

    var actual = {
      height: $(element).height(),
      width: $(element).width()
    };

    $(window).resize(function() {
      actual = {
        height: $(element).height(),
        width: $(element).width()
      };
      scope.$apply();
    });

    scope.X = function(x) {
      return x / projected.width * actual.width;
    };

    scope.Y = function(y) {
      return y / projected.height * actual.height;
    }
  };
});

/* X-coordinate based attributes. Wraps these attributes in a call to the
 * corresponding X() function (see mciCanvas directive) in the element's scope.
 * TODO: 'R', circle radius, really shouldn't be here, but this hack will do
 *       for now.
 */
_.each(['Width', 'X', 'X1', 'X2', 'R', 'Cx'], function(attr) {
  var directiveAttribute = 'mciAttr' + attr;
  var elementAttribute = attr.toLowerCase();
  directives.svg.directive(directiveAttribute, function() {
    return function(scope, element, attrs) {
      scope.$watch('X(' + attrs[directiveAttribute] + ')', function(v) {
        $(element).attr(elementAttribute, v);
      });
    };
  });
});

/* Y-coordinate based attributes. Wraps these attributes in a call to the
 * corresponding X() function (see mciCanvas directive) in the element's scope.
 */
_.each(['Height', 'Y', 'Y1', 'Y2', 'Cy'], function(attr) {
  var directiveAttribute = 'mciAttr' + attr;
  var elementAttribute = attr.toLowerCase();
  directives.svg.directive(directiveAttribute, function() {
    return function(scope, element, attrs) {
      scope.$watch('Y(' + attrs[directiveAttribute] + ')', function(v) {
        $(element).attr(elementAttribute, v);
      });
    };
  });
});