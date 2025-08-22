var directives = directives || {};

directives.eventlogs = angular.module('directives.eventLogs', ['filters.common']);

directives.eventlogs.directive('hostevent', function() {
  return {
    scope:{
      userTz:"=tz",
      eventLogObj:"=event"
    },
    restrict : 'E',
    templateUrl : '/static/partials/hostevent.html',
  };
});

directives.eventlogs.directive('adminevent', function() {
  return {
    scope:{
      userTz:"=tz",
      e:"=event"
    },
    restrict : 'E',
    templateUrl : '/static/partials/adminevent.html',
  };
});
