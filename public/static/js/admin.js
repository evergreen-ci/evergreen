mciModule.controller('AdminSettingsController', ['$scope', '$window', '$http', 'mciAdminRestService', 'notificationService', '$mdpTimePicker', function($scope, $window, $http, mciAdminRestService, notificationService) {
  $scope.auth_is_ldap = $window.auth_is_ldap;

  $scope.load = function() {
    $scope.Settings = {};
    $scope.getSettings();
    $scope.disableRestart = false;
    $scope.disableSubmit = false;
    $scope.restartRed = true;
    $scope.restartPurple = true;
    $scope.restartLavender = true;
    $scope.ValidThemes = [ "announcement", "information", "warning", "important"];
    $("#restart-modal").on("hidden.bs.modal", $scope.enableSubmit);
  }

  $scope.getSettings = function() {
    var successHandler = function(resp) {
      if (resp.data.slack && resp.data.slack.options) {
        var fields = resp.data.slack.options.fields;
        var fieldsSet = [];
        for (var field in fields) {
          fieldsSet.push(field);
        }
        resp.data.slack.options.fields = fieldsSet;
      }

      $scope.tempCredentials = [];
      _.each(resp.data.credentials, function(val, key) {
        var obj = {};
        obj[key] = val;
        $scope.tempCredentials.push(obj);
      });

      $scope.tempExpansions = [];
      _.each(resp.data.expansions, function(val, key) {
        var obj = {};
        obj[key] = val;
        $scope.tempExpansions.push(obj);
      });

      $scope.tempPlugins = resp.data.plugins ? jsyaml.safeDump(resp.data.plugins) : ""
      $scope.tempContainerPools = resp.data.container_pools.pools ? jsyaml.safeDump(resp.data.container_pools.pools) : ""

      // Support transition to region-based EC2Keys struct -- TO BE DELETED
      let default_exists = false;
      if (resp.data.providers.aws.aws_id && resp.data.providers.aws.aws_secret) {
          if (resp.data.providers.aws.ec2_keys) {
              for (var i = 0; i < resp.data.providers.aws.ec2_keys.length; i++) {
                  if (resp.data.providers.aws.ec2_keys[i].region == "us-east-1") { // we already have this region
                    default_exists = true;
                    break;
                  }
              }
          } else {
            resp.data.providers.aws.ec2_keys = [];
          }
          if (!default_exists) {
              resp.data.providers.aws.ec2_keys.push({"region": "us-east-1", "key": resp.data.providers.aws.aws_id, "secret": resp.data.providers.aws.aws_secret})
          }
          resp.data.providers.aws.aws_id = "";
          resp.data.providers.aws.aws_secret = "";
      }

      $scope.Settings = resp.data;
      $scope.Settings.jira_notifications = $scope.Settings.jira_notifications;
      $scope.Settings.jira_notifications.custom_fields = $scope.Settings.jira_notifications.custom_fields || {};
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
      var fields = $scope.Settings.slack.options.fields;
      var fieldsSet = {};
      for (var i = 0; i < fields.length; i++) {
        fieldsSet[fields[i]] = true;
      }
      $scope.Settings.slack.options.fields = fieldsSet;
    }

    $scope.Settings.credentials = {};
    _.map($scope.tempCredentials, function(elem, index) {
      for (var key in elem) {
        $scope.Settings.credentials[key] = elem[key];
      }
    });

    $scope.Settings.expansions = {};
    _.map($scope.tempExpansions, function(elem, index) {
      for (var key in elem) {
        $scope.Settings.expansions[key] = elem[key];
      }
    });

    try {
      $scope.Settings.plugins = jsyaml.safeLoad($scope.tempPlugins);
    } catch(e) {
      notificationService.pushNotification("Error parsing plugin yaml: " + e, "errorHeader");
      return;
    }

    try {
      var parsedContainerPools = jsyaml.safeLoad($scope.tempContainerPools);
    } catch(e) {
      notificationService.pushNotification("Error parsing container pools yaml: " + e, "errorHeader");
      return;
    }

    if (!$scope.tempContainerPools) {
      parsedContainerPools = [];
    }

    // do not save settings if any container pool field is null
    // or if duplicate container pool IDs found
    var uniqueIds = {}
    for (var i = 0; i < parsedContainerPools.length; i++) {
      var p = parsedContainerPools[i]
      // check fields
      if (!p.distro || !p.id || !p.max_containers) {
        notificationService.pushNotification("Error saving settings: container pool field cannot be null", "errorHeader");
        return
      }

      // check uniqueness
      if (p.id in uniqueIds) {
        notificationService.pushNotification("Error saving settings: found duplicate container pool ID: " + p.id , "errorHeader");
        return;
      }
      uniqueIds[p.id] = true
    }

    $scope.Settings.container_pools.pools = parsedContainerPools;

    if ($scope.tempPlugins === null || $scope.tempPlugins === undefined || $scope.tempPlugins == "") {
      $scope.Settings.plugins = {};
    }
    if (!$scope.tempCredentials || $scope.tempCredentials.length === 0) {
      $scope.Settings.credentials = {};
    }
    if (!$scope.tempExpansions || $scope.tempExpansions.length === 0) {
      $scope.Settings.expansions = {};
    }

    mciAdminRestService.saveSettings($scope.Settings, { success: successHandler, error: errorHandler });
  }

  $scope.validEC2Credentials = function(item){
    return item && item.region && item.key && item.secret;
  }

  $scope.addEC2Credential = function(){
    if ($scope.Settings.providers == null || $scope.Settings.providers == undefined) {
      $scope.Settings.providers = {
        "aws": {"ec2_keys": []}
      };
    }
    for (let i = 0; i < $scope.Settings.providers.aws.ec2_keys.length; i++) {
      if ($scope.Settings.providers.aws.ec2_keys[i].region === $scope.new_item.region) {
        $scope.invalidCredential = "Only one key/secret per region.";
          return
      }
    }
      if (!$scope.validEC2Credentials($scope.new_item)){
          $scope.invalidCredential = "EC2 Region, Key, and Secret required.";
          return
      }
      $scope.Settings.providers.aws.ec2_keys.push($scope.new_item);
      $scope.new_item = {};
      $scope.invalidCredential = "";
  }

  $scope.deleteEC2Credential = function(index){
    $scope.Settings.providers.aws.ec2_keys.splice(index, 1);
  }

  $scope.clearAllUserTokens = function(){
    if(!confirm("This will log out all users from all existing sessions. Continue?"))
      return

    $http.post('/admin/cleartokens').then(
      function(resp) {
        window.location.reload();
      },
      function(resp) {
        notificationService.pushNotification("Failed to clear user tokens: " + resp.data.error,'errorHeader');
      });
  }

  $scope.clearCommitQueues = function(){
    if(!confirm("This will clear the contents of all commit queues. Continue?")){
      return
    }

    var successHandler = function(resp) {
      notificationService.pushNotification("Operation successful: cleared " + resp.data.cleared_count + " queues", 'notifyHeader', 'success');
    };
    var errorHandler = function(resp) {
      notificationService.pushNotification("Failed to clear commit queues: " + resp.data.error, 'errorHeader');
    };

    mciAdminRestService.clearCommitQueues({ success: successHandler, error: errorHandler });
  }

  timestamp = function(ts) {
    return "[" + moment(ts, "YYYY-MM-DDTHH:mm:ss").format("lll") + "] ";
  }

  // restartType: tasks or versions
  $scope.restartItems = function(restartType, dryRun) {
    if (restartType === "tasks") {
        var restartTitle = "Tasks";
    } else if (restartType === "versions") {
        var restartTitle = "Commit Queue Versions"
    } else {
        alert("Please choose an item to restart");
        return
    }
    if (!$scope.fromDate || !$scope.toDate || !$scope.toTime || !$scope.fromTime) {
      alert("The from/to date and time must be populated to restart " + restartType);
      return;
    }
    var from = combineDateTime($scope.fromDate, $scope.fromTime);
    var to = combineDateTime($scope.toDate, $scope.toTime);
    if (to < from) {
        alert("From time cannot be after to time");
        $scope.disableSubmit = false;
        return;
    }

    if (restartType === "tasks" && !$scope.restartRed && !$scope.restartPurple && !$scope.restartLavender) {
      alert("No " + restartType + " selected to restart");
      return;
    }
    if (dryRun === false) {
      $scope.disableRestart = true;
      if (restartType === "tasks") {
          var successHandler = function(resp) {
              $("#divMsg").text("The below " + restartType + " have been queued to restart. Feel free to close this popup or inspect the tasks listed.");
              $scope.disableSubmit = false;
          }
      }
      if (restartType === "versions") {
          var successHandler = function(resp) {
              $("#divMsg").text("The below " + restartType + " have been re-added to queue. For more information, please inspect the commit queues.");
              $scope.disableSubmit = false;
          }
      }

    }
    else {
      $scope.disableSubmit = true;
      $scope.disableRestart = false;
      $("#divMsg").text("");
      dryRun = true;
      var successHandler = function(resp) {
        $scope.items = resp.data.items_restarted;
        $scope.modalTitle = "Restart " + restartTitle;
        $scope.itemType = restartType;
        $("#restart-modal").modal("show");
      }
    }
    var errorHandler = function(resp) {
      notificationService.pushNotification("Error restarting " + restartType + ": " + resp.data.error, "errorHeader");
    }

    mciAdminRestService.restartItems(from, to, dryRun, restartType, $scope.restartRed, $scope.restartPurple, $scope.restartLavender, { success: successHandler, error: errorHandler });
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

  // itemType should be task or version
  $scope.jumpToItem = function(itemType, itemId) {
    if (itemType === "versions") {
      window.open("/version/" + itemId);
    }
    else {
      window.open(/task/ + itemId);
    }
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

  $scope.addCredential = function(chip) {
    var obj = {};
    pieces = chip.split(":");
    if (pieces.length !== 2) {
      alert("Input must be in the format of key:value");
      return null;
    }
    var key = pieces[0];
    if ($scope.tempCredentials[key]) {
      alert("Duplicate credential: " + key);
      return null;
    }
    obj[key] = pieces[1];
    $scope.tempCredentials[key] = pieces[1];
    return obj;
  }

  $scope.addExpansion = function(chip) {
    var obj = {};
    pieces = chip.split(":");
    if (pieces.length !== 2) {
      alert("Input must be in the format of key:value");
      return null;
    }
    var key = pieces[0];
    if ($scope.tempExpansions[key]) {
      alert("Duplicate expansion: " + key);
      return null;
    }
    obj[key] = pieces[1];
    $scope.tempExpansions[key] = pieces[1];
    return obj;
  }

  $scope.deleteJIRAProject = function(key) {
    if (!key) {
      return;
    }
    delete $scope.Settings.jira_notifications.custom_fields[key];
  }
  $scope.addJIRAProject = function() {
    var value = $scope.jiraMapping.newProject.toUpperCase();
    if (!value) {
      return;
    }
    if (!$scope.Settings.jira_notifications.custom_fields[value]) {
      $scope.Settings.jira_notifications.custom_fields[value] = {};
    }
    delete $scope.jiraMapping.newProject;
  }
  $scope.addJIRAFieldToProject = function(project) {
    var field = $scope.jiraMapping.newField[project];
    if (!field || $scope.Settings.jira_notifications.custom_fields[project][field]) {
        return;
    }
    $scope.Settings.jira_notifications.custom_fields[project][field] = "{FIXME}";
    delete $scope.jiraMapping.newField[project];
  }
  $scope.deleteJIRAFieldFromProject = function(project, field) {
    if (!field) {
        return;
    }
    delete $scope.Settings.jira_notifications.custom_fields[project][field];
  }

  $scope.jiraMapping = {};

  $scope.load();
}]);
