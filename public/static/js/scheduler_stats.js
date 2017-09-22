mciModule.controller('SchedulerStatsCtrl', function($scope, $http, $window, $filter) {
	var url = '/scheduler/stats/utilization';
	$scope.userTz = $window.userTz;

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

	$scope.utilizationData = {};

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
	$scope.getHostUtilizationData = function(){
		var query = "granularity=" + encodeURIComponent($scope.currentGranularity.value) +
		"&numberDays=" + encodeURIComponent($scope.currentNumberDays.value)
		$http.get(url + "?" + query).then(
		function(resp){
			$scope.utilizationData = resp.data.reverse();
		},
		function(resp){
			console.log(resp.status)
		});
	};

	$scope.setNumberDays = function(numberDays){
		$scope.currentNumberDays = numberDays;
		$scope.getHostUtilizationData();
	}
	$scope.setGranularity = function(granularity){
		$scope.currentGranularity = granularity;
		$scope.getHostUtilizationData();
	}

	$scope.getPercentUtilization = function(data){
		if (data.static_host.total_time == 0 && data.dynamic_host.total_time == 0){
			return 0;
		}
		return (data.task/(data.static_host + data.dynamic_host)* 100).toFixed(2);
	};


	$scope.getHostUtilizationData();
});
