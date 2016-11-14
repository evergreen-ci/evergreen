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

directives.eventlogs.directive('distroevent', function() {
  return {
    scope:{
      userTz:"=tz",
      e:"=event"
    },
    restrict : 'E',
    templateUrl : '/static/partials/distroevent.html',
  };
});


