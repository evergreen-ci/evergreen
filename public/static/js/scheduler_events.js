mciModule.controller('SchedulerEventCtrl', function($scope, $window) {
	$scope.events = $window.events;
	$scope.userTz = $window.userTz;
	$scope.distro = $window.distro;

	$scope.fullEvents = _.filter($scope.events, function(event){
			return event.data.task_queue_info.task_queue_length > 0;
		});
})