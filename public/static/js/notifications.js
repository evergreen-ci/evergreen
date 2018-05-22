mciModule.controller('NotificationsController', function($scope, $window, mciUserSettingsService, mciSubscriptionsService, notificationService) {
  $scope.load = function() {
    $scope.getUserSettings();
    $scope.getSubscriptions();
  };

  $scope.getUserSettings = function() {
    var success = function(resp) {
      $scope.settings = resp.data;
    };
    var failure = function(resp) {
      notificationService.pushNotification("Failed to get settings: " + resp.data,'errorHeader');
    };
    mciUserSettingsService.getUserSettings({success: success, error: failure});
  };

  $scope.getSubscriptions = function() {
    var success = function(resp) {
      $scope.subscriptions = resp.data;
    };
    var failure = function(resp) {
      notificationService.pushNotification("Failed to get subscriptions: " + resp.data,'errorHeader');
    };
    mciSubscriptionsService.get(user, "person", {success: success, error: failure});
  };

  $scope.updateUserSettings = function() {
    var success = function() {
      window.location.reload();
    };
    var failure = function(resp) {
      notificationService.pushNotification("Failed to save changes: " + resp.data,'errorHeader');
    };
    mciUserSettingsService.saveUserSettings($scope.settings, {success: success, error: failure});
  };

  $scope.deleteSubscription = function(id) {
    var success = function() {
      $scope.getSubscriptions();
    };
    var failure = function(resp) {
      notificationService.pushNotification("Failed to delete subscription: " + resp.data,'errorHeader');
    };
    mciSubscriptionsService.delete(id, {success: success, error: failure});
  }

  $scope.subscriberText = function(input) {
    switch (input.type) {
    case "jira-issue":
      return "make a Jira issue in " + input.target;
    case "jira-comment":
      return "make a comment on Jira issue " + input.target;
    case "evergreen-webhook":
      return "post to server " + input.target;
    case "email":
      return "email " + input.target;
    case "slack":
      return "send a Slack message to " + input.target;
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
