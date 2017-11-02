mciModule.controller('AdminSettingsController', ['$scope','$window', 'mciAdminRestService', 'notificationService', '$mdpTimePicker', function($scope, $window, mciAdminRestService, notificationService) {
  $scope.load = function() {
    $scope.Settings = {};
    $scope.Events = generateEventText(window.events);
    $scope.getSettings();
    $scope.disableRestart = false;
    $scope.disableSubmit = false;
    $scope.restartRed = true;
    $scope.restartPurple = true;
    $scope.ValidThemes = [ "announcement", "information", "warning", "important"];
    $("#tasks-modal").on("hidden.bs.modal", $scope.enableSubmit);
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
        case "THEME_CHANGED":
          event.displayText = themeChangeEventText(event);
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

  themeChangeEventText = function(event) {
    var oldVal = event.data.old_val ? "'"+event.data.old_val+"'" : "(blank)";
    var newVal = event.data.new_val ? "'"+event.data.new_val+"'" : "(blank)";
    return timestamp(event.timestamp) + event.data.user + " changed banner theme from " + oldVal + " to " + newVal;
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

  $scope.restartTasks = function(dryRun) {
    if (!$scope.fromDate || !$scope.toDate || !$scope.toTime || !$scope.fromTime) {
      alert("The from/to date and time must be populated to restart tasks");
      return;
    }
    if (!$scope.restartRed && !$scope.restartPurple) {
      alert("No tasks selected to restart");
      return;
    }
    if (dryRun === false) {
      $scope.disableRestart = true;
      var successHandler = function(resp) {
        $("#divMsg").text("The below tasks have been queued to restart. Feel free to close this popup or inspect the tasks listed.");
        $scope.disableSubmit = false;
      }
    }
    else {
      $scope.disableSubmit = true;
      $scope.disableRestart = false;
      $("#divMsg").text("");
      dryRun = true;
      var successHandler = function(resp) {
        $scope.tasks = resp.data.tasks_restarted;
        $scope.modalTitle = "Restart Tasks";
        $("#tasks-modal").modal("show");
      }
    }
    var errorHandler = function(resp) {
      notificationService.pushNotification("Error restarting tasks: " + resp.data.error, "errorHeader");
    }
    var from = combineDateTime($scope.fromDate, $scope.fromTime);
    var to = combineDateTime($scope.toDate, $scope.toTime);
    if (to < from) {
      alert("From time cannot be after to time");
      $scope.disableSubmit = false;
      return;
    }
    mciAdminRestService.restartTasks(from, to, dryRun, $scope.restartRed, $scope.restartPurple, { success: successHandler, error: errorHandler });
  }

  combineDateTime = function(date, time) {
    date.setHours(time.getHours());
    date.setMinutes(time.getMinutes());

    return date;
  }

  $scope.enableSubmit = function() {
    $scope.disableSubmit = false;
    $scope.$apply();
  }

  $scope.jumpToTask = function(taskId) {
    window.open("/task/" + taskId);
  }

  $scope.load();
}]);
