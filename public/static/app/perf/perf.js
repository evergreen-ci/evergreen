mciModule.controller('PerfController', function PerfController(
  $scope, $window, $http, $location, $filter, ChangePointsService, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, $sce,
) {
  $scope.task = $window.task_data;
  $scope.newTrendChartsUi = $sce.trustAsResourceUrl(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.UI + "/task/" + ($scope.task ? $scope.task.id : null) + "/performanceData");
});
