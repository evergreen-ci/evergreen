var directives = directives || {};

directives.badges = angular.module('directives.badges', ['filters.common']);

directives.badges.directive('buildStatusLabel', function($filter) {
  return function(scope, element, attrs) {
    // buildStatusLabel should be a particular build, and we adjust
    // the label to match the state of the build
    scope.$watch(attrs.buildStatusLabel, function(v, old) {
      if (v) {
        switch (v.Build.status) {
          case 'created':
          case 'unstarted':
            if (v.Build.activated) {
              element.html('Scheduled');
            } else {
              element.html('Not Scheduled');
            }
            break;
          case 'started':
            element.html('In Progress');
            break;
          default:
            element.html($filter('capitalize')(v.Build.status));
            break;
        }
      }
    });
  };
});
