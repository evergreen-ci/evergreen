mciModule.controller('AdminSettingsController', ['$scope','$window', 'mciAdminRestService', 'notificationService', function($scope, $window, mciAdminRestService, notificationService) {
  $scope.load = function() {
    $scope.Settings = {};
    $scope.Events = generateEventText(window.events);
    $scope.getSettings();
  }

  $scope.getSettings = function() {
    var successHandler = function(resp) {
      $scope.Settings = resp.data;
    }
    var errorHandler = function(resp) {
      notificationService.pushNotification("Error loading settings: " + resp.data.error, "errorHeader");
    }
    mciAdminRestService.getSettings({ success: successHandler, error: errorHandler });
  }

  $scope.saveSettings = function() {
    var successHandler = function(resp) {
      window.location.href = "/admin";
    }
    var errorHandler = function(resp) {
      notificationService.pushNotification("Error saving settings: " + resp.data.error, "errorHeader");
    }
    mciAdminRestService.saveSettings($scope.Settings, { success: successHandler, error: errorHandler });
  }

  generateEventText = function(events) {
    for (var i = 0; i < events.length; i++) {
      var event = events[i];
      switch(event.event_type) {
        case "BANNER_CHANGED":
          event.displayText = bannerChangeEventText(event);
          break;
        case "SERVICE_FLAGS_CHANGED":
          event.displayText = flagChangeEventText(event);
          break;
      }
    }
    return events;
  }

  var flagDisplayNames = {
    task_dispatch_disabled: "task dispatch",
    hostinit_disabled: "hostinit",
    monitor_disabled: "monitor",
    notifications_disabled: "notifications",
    alerts_disabled: "alerts",
    taskrunner_disabled: "taskrunner",
    repotracker_disabled: "repotracker",
    scheduler_disabled: "scheduler"
  }

  bannerChangeEventText = function(event) {
    var oldVal = event.data.old_val ? "'"+event.data.old_val+"'" : "(blank)";
    var newVal = event.data.new_val ? "'"+event.data.new_val+"'" : "(blank)";
    return timestamp(event.timestamp) + event.data.user + " changed banner from " + oldVal + " to " + newVal;
  }

  flagChangeEventText = function(event) {
    var changes = [];
    for (var flag in flagDisplayNames) {
      var newVal = event.data.new_flags[flag];
      var oldVal = event.data.old_flags[flag];
      if (oldVal !== newVal) {
        changes.push((newVal?" disabled ":" enabled ") + flagDisplayNames[flag]);
      }
    }
    return timestamp(event.timestamp) + event.data.user + changes.join(", ");
  }

  timestamp = function(ts) {
    return "[" + moment(ts, "YYYY-MM-DDTHH:mm:ss").format("lll") + "] ";
  }

  $scope.load();
}]);
