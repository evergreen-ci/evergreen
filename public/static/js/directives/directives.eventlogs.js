var directives = directives || {};

directives.eventlogs = angular.module('directives.eventLogs', ['filters.common']);

directives.eventlogs.directive('taskevent', function() {
  return {
    scope:{
      userTz:"=tz",
      eventLogObj:"=event"
    },
    restrict : 'E',
    templateUrl : '/static/partials/taskevent.html',
  };
});

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

