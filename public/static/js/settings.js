mciModule.controller('SettingsCtrl', ['$scope', '$http', '$window', 'notificationService', 'mciUserSettingsService', "timeUtil", function ($scope, $http, $window, notifier, mciUserSettingsService, timeUtil) {
  $scope.timezones = timeUtil.timezones;

  $scope.patch_feedback_prompts = {
    "information_score": "Does the new patches page have all the information you need?",
    "usability_score": "How easy is it to use the new page compared to the old page?",
    "missing_things": "Is there anything you miss about the old patches page?",
    "requested_changes": "Is there anything you want changed on the new patches page?",
  };

  $scope.user_tz = $window.user_tz;
  $scope.new_tz = $scope.user_tz || "America/New_York";
  $scope.new_region = $window.user_region || "us-east-1";
  $scope.github_user = $window.github_user;
  $scope.use_spruce_options = $window.use_spruce_options;
  $scope.should_show_feedback = false;
  $scope.opt_in_initially_checked = $scope.use_spruce_options === undefined ? false : $scope.use_spruce_options.patch_page;
  $scope.userConf = $window.userConf;
  $scope.binaries = $window.binaries;
  $scope.notifications = $window.notifications;
  $scope.slack_username = $window.slack_username;
  $scope.can_clear_tokens = $window.can_clear_tokens;
  $scope.spruce_feedback = {};

  $scope.newKey = function () {
    if (!confirm("Generating a new API key will invalidate your current API key. Continue?"))
      return

    $http.post('/settings/newkey').then(
      function (resp) {
        var data = resp.data;
        $scope.userConf.api_key = data.key
        $scope.selectConf()
      },
      function (resp) {
        console.log(resp.data, resp.status);
      });
  }

  $scope.clearToken = function () {
    if (!confirm("This will log you out from all existing sessions. Continue?"))
      return

    $http.post('/settings/cleartoken').then(
      function (resp) {
        window.location.reload();
      },
      function (resp) {
        notifier.pushNotification("Failed to clear user token: " + resp.data.error, 'errorHeader');
      });
  }

  $scope.onOptOutChange = function () {
    if ($scope.opt_in_initially_checked && !$scope.use_spruce_options.patch_page) {
      $scope.should_show_feedback = true;
    } else {
      $scope.should_show_feedback = false;
    }
  }

  function formatFeedback(spruce_feedback) {
    var formattedFeedback = {
      type: "new_patches_page_feedback"
    };
    var allFields = [];
    var questionAnswerArray = [];
    Object.keys($scope.patch_feedback_prompts).forEach(function (field) {
      var prompt = $scope.patch_feedback_prompts[field];
      var answer = spruce_feedback[field] === undefined ? "" : spruce_feedback[field].toString();
      var questionAnswer = {
        id: field,
        prompt: prompt,
        answer: answer
      };
      questionAnswerArray.push(questionAnswer);
    });
    formattedFeedback["questions"] = questionAnswerArray;
    return formattedFeedback;
  }

  $scope.updateUserSettings = function (new_tz, new_region, use_spruce_options, spruce_feedback) {
    if ($scope.opt_in_initially_checked && !use_spruce_options.patch_page &&
      (spruce_feedback.usability_score === undefined || spruce_feedback.information_score === undefined)) {
      notifier.pushNotification("Please fill out all required fields before submitting", 'errorHeader');
      return;
    }
    data = {
      timezone: new_tz,
      region: new_region,
      use_spruce_options: use_spruce_options,
      github_user: {
        last_known_as: $scope.github_user,
      }
    };
    if ($scope.opt_in_initially_checked && !use_spruce_options.patch_page) {
      data.spruce_feedback = formatFeedback(spruce_feedback);
    }
    var success = function () {
      window.location.reload();
    };
    var failure = function (resp) {
      notifier.pushNotification("Failed to save changes: " + resp.data.error, 'errorHeader');
    };
    mciUserSettingsService.saveUserSettings(data, {
      success: success,
      error: failure
    });
  };
}]);