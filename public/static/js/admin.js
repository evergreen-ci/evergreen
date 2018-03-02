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
      if (resp.data.slack && resp.data.slack.options) {
        let fields = resp.data.slack.options.fields;
        let fieldsSet = [];
        for (var field in fields) {
          fieldsSet.push(field);
        }
        resp.data.slack.options.fields = fieldsSet;
      }

      $scope.tempCredentials = [];
      _.each(resp.data.credentials, function(val, key) {
        let obj = {};
        obj[key] = val;
        $scope.tempCredentials.push(obj);
      });

      $scope.tempExpansions = [];
      _.each(resp.data.expansions, function(val, key) {
        let obj = {};
        obj[key] = val;
        $scope.tempExpansions.push(obj);
      });

      $scope.tempPlugins = jsyaml.safeDump(resp.data.plugins);

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

    if ($scope.Settings.slack && $scope.Settings.slack.options) {
      let fields = $scope.Settings.slack.options.fields;
      let fieldsSet = {};
      for (var i = 0; i < fields.length; i++) {
        fieldsSet[fields[i]] = true;
      }
      $scope.Settings.slack.options.fields = fieldsSet;
    }

    _.map($scope.tempCredentials, function(elem, index) {
      for (var key in elem) {
        $scope.Settings.credentials[key] = elem[key];
      }
    });

    _.map($scope.tempExpansions, function(elem, index) {
      for (var key in elem) {
        $scope.Settings.expansions[key] = elem[key];
      }
    });

    try {
      $scope.Settings.plugins = jsyaml.safeLoad($scope.tempPlugins);
    } catch(e) {
      alert("Error parsing plugin yaml: " + e);
      return;
    }

    mciAdminRestService.saveSettings($scope.Settings, { success: successHandler, error: errorHandler });
  }

  generateEventText = function(events) {
    if (!events) { return; }
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
    scheduler_disabled: "scheduler",
    github_pr_testing_disabled: "github_pr_testing",
    repotracker_push_event_disabled: "repotracker_push_event",
    cli_updates_disabled: "cli_updates",
    github_status_api_disabled: "github_status_api"
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

  $scope.scrollTo = function(section) {
    var offset = $('#'+section).offset();
    var scrollto = offset.top - 55; //position of the element - header height(ish)
    $('html, body').animate({scrollTop:scrollto}, 0);
  }

  $scope.clearSection = function(section, subsection) {
    if (!subsection) {
      $scope.Settings[section] = {};
    } else {
      $scope.Settings[section][subsection] = {};
    }
  }

  $scope.transformNaiveUser = function(chip) {
    var user = {};
    try {
      var user = JSON.parse(chip);
    } catch(e) {
      alert("Unable to parse json: " + e);
      return null;
    }
    if (!user.username || user.username === "") {
      alert("You must enter a username");
      return null;
    }

    return user;
  }

  $scope.addKVpair = function(chip, property) {
    var obj = {};
    pieces = chip.split(":");
    if (pieces.length !== 2) {
      alert("Input must be in the format of key:value");
      return null;
    }
    var key = pieces[0];
    if ($scope.Settings[property][key]) {
      alert("Duplicate key: " + key);
      return null;
    }
    obj[key] = pieces[1];
    $scope.Settings[property][key] = pieces[1];
    return obj;
  }

  $scope.load();
}]);
