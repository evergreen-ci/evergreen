mciModule.controller("SettingsCtrl", [
  "$scope",
  "$http",
  "$window",
  "notificationService",
  "mciUserSettingsService",
  "timeUtil",
  function (
    $scope,
    $http,
    $window,
    notifier,
    mciUserSettingsService,
    timeUtil
  ) {
    $scope.timezones = timeUtil.timezones;

    $scope.user_tz = $window.user_tz;
    $scope.new_tz = $scope.user_tz || "America/New_York";
    $scope.new_region = $window.user_region || "us-east-1";
    $scope.github_user = $window.github_user;
    $scope.use_spruce_options = $window.use_spruce_options;
    $scope.should_show_feedback = false;
    $scope.opt_in_initially_checked =
      $scope.use_spruce_options === undefined
        ? false
        : $scope.use_spruce_options.patch_page;
    $scope.userConf = $window.userConf;
    $scope.binaries = $window.binaries;
    $scope.notifications = $window.notifications;
    $scope.slack_username = $window.slack_username;
    $scope.can_clear_tokens = $window.can_clear_tokens;
    $scope.spruce_feedback = {};

    $scope.newKey = function () {
      if (
        !confirm(
          "Generating a new API key will invalidate your current API key. Continue?"
        )
      )
        return;

      $http.post("/settings/newkey").then(
        function (resp) {
          var data = resp.data;
          $scope.userConf.api_key = data.key;
          $scope.selectConf();
        },
        function (resp) {
          console.log(resp.data, resp.status);
        }
      );
    };

    $scope.clearToken = function () {
      if (
        !confirm("This will log you out from all existing sessions. Continue?")
      )
        return;

      $http.post("/settings/cleartoken").then(
        function (resp) {
          window.location.reload();
        },
        function (resp) {
          notifier.pushNotification(
            "Failed to clear user token: " + resp.data.error,
            "errorHeader"
          );
        }
      );
    };

    $scope.updateUserSettings = function () {
      data = {
        timezone: $scope.new_tz,
        region: $scope.new_region,
        github_user: {
          last_known_as: $scope.github_user,
        },
      };
      var success = function () {
        window.location.reload();
      };
      var failure = function (resp) {
        notifier.pushNotification(
          "Failed to save changes: " + resp.data.error,
          "errorHeader"
        );
      };
      mciUserSettingsService.saveUserSettings(data, {
        success: success,
        error: failure,
      });
    };
  },
]);
