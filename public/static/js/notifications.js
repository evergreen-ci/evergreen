mciModule.controller('NotificationsController', function($scope, $window, mciUserSettingsService, mciSubscriptionsService, notificationService) {
  $scope.load = function() {
    $scope.getData();
  };

  $scope.getData = function() {
    var success = function(resp) {
      $scope.settings = resp.data;
      $scope.getSubscriptions();
    };
    var failure = function(resp) {
      notificationService.pushNotification("Failed to get settings: " + resp.data,'errorHeader');
    };
    mciUserSettingsService.getUserSettings({success: success, error: failure});
  };

  $scope.getSubscriptions = function() {
    var success = function(resp) {
      var patchFinishId = $scope.settings.notifications.patch_finish_id;
      var buildBreakId = $scope.settings.notifications.build_break_id;
      var spawnhostExpirationId = $scope.settings.notifications.spawn_host_expiration_id;
      var commitQueueId = $scope.settings.notifications.commit_queue_id;
      if (!Array.isArray(resp.data)) {
        resp.data = [resp.data];
      }
      $scope.subscriptions = _.filter(resp.data, function(subscription){
        if (
          subscription.id === patchFinishId || 
          subscription.id === buildBreakId || 
          subscription.id === spawnhostExpirationId || 
          subscription.id === commitQueueId) {
          return false;
        }
        return true;
      });
    };
    var failure = function(resp) {
      notificationService.pushNotification("Failed to get subscriptions: " + resp.data.error, 'errorHeader');
    };
    mciSubscriptionsService.get(user, "person", {success: success, error: failure});
  };

  $scope.updateUserSettings = function() {
    var success = function() {
      window.location.reload();
    };
    var failure = function(resp) {
      notificationService.pushNotification("Failed to save changes: " + resp.data.error, 'errorHeader');
    };
    mciUserSettingsService.saveUserSettings($scope.settings, {success: success, error: failure});
  };

  $scope.clearAllSubscriptions = function() {
    $scope.subscriptions.forEach(function(sub) {
      $scope.deleteSubscription(sub.id);
    });
  }

  $scope.deleteSubscription = function(id) {
    var success = function() {
      $scope.getSubscriptions();
    };
    var failure = function(resp) {
      notificationService.pushNotification("Failed to delete subscription: " + resp.data.error, 'errorHeader');
    };
    mciSubscriptionsService.delete(id, {success: success, error: failure});
  }

  $scope.subscriberText = function(input) {
    switch (input.type) {
    case "jira-issue":
      return "making a Jira issue in " + input.target.project + ' with issue type ' + input.target.issue_type;
    case "jira-comment":
      return "making a comment on Jira issue " + input.target;
    case "evergreen-webhook":
      return "posting to server " + input.target;
    case "email":
      return "emailing " + input.target;
    case "slack":
      return "sending a Slack message to " + input.target;
    }
    return input;
  };

  $scope.selectorText = function(input) {
    var selector = parseSelector(input);
    var out = selector.model;
    if (selector.status) {
      out += " in status " + selector.status;
    }
    if (selector.project) {
      out += " in project " + selector.status;
    }
    return out;
  }

  $scope.selectorClick = function(input) {
    var selector = parseSelector(input[0][0]);
    var link = "/" + selector.model + "/" + selector.id;
    window.open(link);
  }

  parseSelector = function(selectors) {
    var parsed = {};
    _.each(selectors, function(selector) {
      switch (selector.type) {
      case "object":
        parsed.model = selector.data;
        break;
      case "id":
        parsed.id = selector.data;
        break;
      case "status":
        parsed.status = selector.data;
        break;
      case "project":
        parsed.project = selector.data;
        break;
      }
    });
    return parsed;
  }

  $scope.load();
});
