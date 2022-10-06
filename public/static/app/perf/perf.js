mciModule.controller('PerfController', function PerfController(
  $scope, $window, $http, $location, $filter, PERFORMANCE_ANALYSIS_AND_TRIAGE_API, $sce,
) {
  $scope.conf = $window.plugins["perf"];
  $scope.task = $window.task_data;
  $scope.newTrendChartsUi = $sce.trustAsResourceUrl(PERFORMANCE_ANALYSIS_AND_TRIAGE_API.UI + "/task/" + ($scope.task ? $scope.task.id : null) + "/performanceData");
});
