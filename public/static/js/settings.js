mciModule.controller('SettingsCtrl', ['$scope', '$http', '$window', 'errorPasserService', function($scope, $http, $window, errorPasser) {
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

  $scope.user_tz = $window.user_tz;
  $scope.new_tz = $scope.user_tz || "America/New_York";
  $scope.userConf = $window.userConf;
  $scope.binaries = $window.binaries;

  $scope.newKey = function(){
    if(!confirm("Generating a new API key will invalidate your current API key. Continue?"))
      return

    $http.post('/settings/newkey').
      success(function(data, status) {
        $scope.userConf.api_key = data.key
        $scope.selectConf()
      }).
      error(function(data, status, errorThrown) {
        console.log(data,status);
      });
  }

  $scope.updateUserSettings = function(new_tz) {
    data = {timezone: new_tz};
    $http.put('/settings/', data)
      .success(function(data, status) {
        window.location.reload()
      })
      .error(function(jqXHR, status, errorThrown) {
        errorPasser.pushError("Failed to save changes: " + jqXHR,'errorHeader')
      });
   };
}]);
