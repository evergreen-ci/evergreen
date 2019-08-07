mciModule.controller('SettingsCtrl', ['$scope', '$http', '$window', 'notificationService', 'mciUserSettingsService', function($scope, $http, $window, notifier, mciUserSettingsService) {
  $scope.timezones = [
    {str: "American Samoa, Niue", value: "Pacific/Niue"},
    {str: "Hawaii", value: "Pacific/Tahiti"},
    {str: "Marquesas Islands", value: "Pacific/Marquesas"},
    {str: "Alaska", value: "America/Anchorage"},
    {str: "Pacific Time", value: "America/Vancouver"},
    {str: "Mountain Time", value: "America/Denver"},
    {str: "Central Time", value: "America/Chicago"},
    {str: "Eastern Time", value: "America/New_York"},
    {str: "Venezuela", value: "America/Caracas"},
    {str: "Atlantic Time", value: "America/Barbados"},
    {str: "Newfoundland", value: "America/St_Johns"},
    {str: "Argentina, Paraguay", value: "America/Belem"},
    {str: "Fernando de Noronha", value: "America/Noronha"},
    {str: "Cape Verde", value: "Atlantic/Cape_Verde"},
    {str: "Iceland", value: "Atlantic/Reykjavik"},
    {str: "United Kingdom, Ireland", value: "Europe/London"},
    {str: "Central European Time, Nigeria", value: "Europe/Rome"},
    {str: "Egypt, Israel, Romania", value: "Europe/Bucharest"},
    {str: "Ethiopia, Iraq, Yemen", value: "Asia/Baghdad"},
    {str: "Iran", value: "Asia/Tehran"},
    {str: "Dubai, Moscow", value: "Europe/Moscow"},
    {str: "Afghanistan", value: "Asia/Kabul"},
    {str: "Maldives, Pakistan", value: "Antarctica/Davis"},
    {str: "India, Sri Lanka", value: "Asia/Kolkata"},
    {str: "Nepal", value: "Asia/Kathmandu"},
    {str: "Bangladesh, Bhutan", value: "Asia/Dhaka"},
    {str: "Cocos Islands, Myanmar", value: "Asia/Rangoon"},
    {str: "Thailand, Vietnam", value: "Asia/Bangkok"},
    {str: "China, Hong Kong, Perth", value: "Asia/Hong_Kong"},
    {str: "Eucla (Unofficial)", value: "Australia/Eucla"},
    {str: "Japan, South Korea", value: "Asia/Seoul"},
    {str: "Australia Central Time", value: "Australia/Adelaide"},
    {str: "Australia Eastern Time", value: "Australia/Sydney"},
    {str: "Lord Howe Island", value: "Australia/Lord_Howe"},
    {str: "Russia Vladivostok Time", value: "Asia/Vladivostok"},
    {str: "Norfolk Island", value: "Pacific/Norfolk"},
    {str: "Fiji, Russia Magadan Time", value: "Asia/Magadan"},
    {str: "Chatham Islands", value: "Pacific/Chatham"},
    {str: "Tonga", value: "Pacific/Tongatapu"},
    {str: "Kiribati Line Islands", value: "Pacific/Kiritimati"},
  ];

  $scope.patch_feedback_prompts = {
    "information_score":  "Does the new patches page have all the information you need?",
    "usability_score": "How easy is it to use the new page compared to the old page?",
    "missing_things": "Is there anything you miss about the old patches page?",
    "requested_changes": "Is there anything you want changed on the new patches page?",
  };

  $scope.user_tz = $window.user_tz;
  $scope.new_tz = $scope.user_tz || "America/New_York";
  $scope.github_user = $window.github_user;
  $scope.use_spruce_options = $window.use_spruce_options;
  $scope.should_show_feedback = false;
  $scope.opt_in_initially_checked = $scope.use_spruce_options === undefined ? false : $scope.use_spruce_options;
  $scope.userConf = $window.userConf;
  $scope.binaries = $window.binaries;
  $scope.notifications = $window.notifications;
  $scope.slack_username = $window.slack_username;
  $scope.auth_is_ldap = $window.auth_is_ldap;
  $scope.spruce_feedback = {};

  $scope.newKey = function(){
    if(!confirm("Generating a new API key will invalidate your current API key. Continue?"))
      return

    $http.post('/settings/newkey').then(
      function(resp) {
        var data = resp.data;
        $scope.userConf.api_key = data.key
        $scope.selectConf()
      },
      function(resp) {
        console.log(resp.data,resp.status);
      });
  }

  $scope.clearToken = function(){
    if(!confirm("This will log you out from all existing sessions. Continue?"))
      return

    $http.post('/settings/cleartoken').then(
      function(resp) {
        window.location.reload();
      },
      function(resp) {
        notifier.pushNotification("Failed to clear user token: " + resp.data.error,'errorHeader');
      });
  }

  $scope.onOptOutChange = function(){
    if($scope.opt_in_initially_checked && !$scope.use_spruce_options.patch_page) {
      $scope.should_show_feedback = true;
    } else {
      $scope.should_show_feedback = false;
    }
  }

  function formatFeedback(spruce_feedback) {
    var formattedFeedback = { type: "new_patches_page_feedback" };
    var allFields = [];
    var questionAnswerArray = [];
    Object.keys($scope.patch_feedback_prompts).forEach( function(field) {
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

  $scope.updateUserSettings = function(new_tz, use_spruce_options, spruce_feedback) {
    if ($scope.opt_in_initially_checked && !use_spruce_options.patch_page &&
      (spruce_feedback.usability_score === undefined || spruce_feedback.information_score === undefined)) {
      notifier.pushNotification("Please fill out all required fields before submitting",'errorHeader');
      return;
    }
    data = {
        timezone: new_tz,
        use_spruce_options: use_spruce_options,
        spruce_feedback: formatFeedback(spruce_feedback),
        github_user: {
            last_known_as: $scope.github_user,
        }
    };
    var success = function() {
      window.location.reload();
    };
    var failure = function(resp) {
      notifier.pushNotification("Failed to save changes: " + resp.data.error,'errorHeader');
    };
    mciUserSettingsService.saveUserSettings(data, {success: success, error: failure});
   };
}]);
