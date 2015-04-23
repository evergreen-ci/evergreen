function TimelineController($scope, $timeline, $window, $location) {
  $scope.data = $timeline;
  $scope.userTz = $window.userTz;

  $scope.currentPage = $location.search()['page'] || 0;
  $scope.numPerPage = 10;

  $scope.loadCurrentPage = function() {
    $timeline.loadPage($window.project, $scope.numPerPage, $scope.currentPage);
    $location.search('page', $scope.currentPage);
  };

  $scope.loadCurrentPage();

  $scope.getLastPageIndex = function() {
    return Math.ceil($timeline.TotalVersions / $scope.numPerPage) - 1;
  };

  $scope.firstPage = function() {
    $scope.currentPage = 0;
    $scope.loadCurrentPage();
  };

  $scope.previousPage = function() {
    $scope.currentPage = Math.max($scope.currentPage - 1, 0);
    $scope.loadCurrentPage();
  };

  $scope.nextPage = function() {
    var lastPage = $scope.getLastPageIndex();
    $scope.currentPage = Math.min($scope.currentPage + 1, lastPage);
    $scope.loadCurrentPage();
  };

  $scope.lastPage = function() {
    var lastPage = $scope.getLastPageIndex();
    $scope.currentPage = lastPage;
    $scope.loadCurrentPage();
  };

  $scope.versionActivated = function(version) {
    return _.chain(version.build_variants_status)
           // TODO, use _.property if we ever update underscore
           .map(function(bvs) { return bvs.activated; })
           .some()
           .value()
  };
}
