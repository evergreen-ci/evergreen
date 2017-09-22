mciModule.controller('SchedulerEventCtrl', function($scope, $window, $http) {
  $scope.events = [];
  $scope.stats = [];
  $scope.userTz = $window.userTz;
  $scope.distro = $window.distro;


  $scope.consts = {
    logs : "logs",
    stats: "stats"
  };

  // granularitySeconds represents the amount of seconds in a given bucket
  $scope.granularitySeconds = [
  {display: "day", value: 24 * 60 * 60, format:"MM/DD"},
  {display: "hour", value: 60 * 60, format:"H:mm"},
  {display: "minute", value: 60, format:"H:mm"},
  ];
  $scope.currentGranularity = $scope.granularitySeconds[1];

  $scope.numberDays = [
  {display: "1 day", value: "1"},
  {display: "1 week", value: 7},
  {display: "2 weeks", value: 14},
  {display: "1 month",value: 30},
  {display: "2 Months", value: 60},
  {display: "3 months", value: 90}
  ];

  $scope.currentNumberDays = $scope.numberDays[0];

  // disableDays sets the buttons to be disabled if the the granularity is a minute and
  // there are too many days to load.
  $scope.disableDays = function(numberDays){
    if ($scope.currentGranularity.display== "minute") {
      if (numberDays.value >= 30) {
        return true;
      }
    }
    return false;
  }


  $scope.setNumberDays = function(numberDays){
    $scope.currentNumberDays = numberDays;
    $scope.stats = [];
    $scope.loadData();
  }
  $scope.setGranularity = function(granularity){
    $scope.currentGranularity = granularity;
    $scope.stats = [];
    $scope.loadData();
  }

  $scope.loadData = function(){
    switch($scope.tab){
      case $scope.consts.logs:
        if ($scope.events.length == 0){
          $http.get("/scheduler/distro/" + $scope.distro + "/logs").then(
          function(resp){
            var data = resp.data;
            $scope.events = data;
            $scope.fullEvents = _.filter($scope.events, function(event){
              return event.data.task_queue_info.task_queue_length > 0;
            });
            return
          },
          function(resp){
            console.log(resp.status);
          });
        }
        break;
      case $scope.consts.stats:
        if ($scope.stats.length == 0) {
          var query = "granularity=" + encodeURIComponent($scope.currentGranularity.value) +
          "&numberDays=" + encodeURIComponent($scope.currentNumberDays.value);
          $http.get("/scheduler/distro/"+ $scope.distro + "/stats?" + query).then(
          function(resp){
            $scope.stats = resp.data;
          },
          function(resp){
            console.log(resp.status);
          });
        }
        break;
      }
  }

  $scope.tab = $scope.consts.logs;
  $scope.loadData();

  $scope.setTab = function(tabName){
    $scope.tab = tabName;
    $scope.loadData();
  }

})
