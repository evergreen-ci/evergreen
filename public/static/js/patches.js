mciModule.controller('PatchesController', function($scope, $filter, $http, $window,
  $location, $rootScope) {
  $scope.userTz = $window.userTz;

  $scope.loading = true;
  $scope.patchesForUsername = $window.patchesForUsername;
  var endpoint = $scope.patchesForUsername ?
    '/json/patches/user/' + encodeURIComponent($window.patchesForUsername) :
    '/json/patches/project/' + encodeURIComponent($scope.project);

  $scope.previousPage = function() {
    $location.search('page', Math.max(0, $scope.currentPage - 1));
  };

  $scope.nextPage = function() {
    $location.search('page', $scope.currentPage + 1);
  };

  $scope.loadCurrentPage = function() {
    $scope.filterCommitQueue = localStorage.getItem("filterCommitQueue") === "true";
    $scope.loading = true;
    $scope.patches = [];
    $scope.patchesError = null;
    var params = {
      params: {
        page: $scope.currentPage,
        filter_commit_queue: $scope.filterCommitQueue,
      }
    };
    $http.get(endpoint, params).then(
    function(resp) {
      var data = resp.data;
      $scope.loading = false;
      $scope.buildsMap = data['BuildsMap'];
      $scope.patches = data['UIPatches'];

      _.each($scope.patches, function(patch) {
          patch.canEdit = (($window.user.Id === patch.author ) || $window.isSuperUser) && patch.alias !== "__commit_queue"
      });

      _.each($scope.buildsMap, function(buildArray) {
        _.each(buildArray, function(build) {
          build.taskResults = [];
          _.each(build.tasks, function(task) {
            build.taskResults.push({
              link: '/task/' + task.id,
              tooltip: task.display_name,
              'class': $filter('statusFilter')(task),
            });
          });
        });
      });
    },
    function(resp) {
      $scope.loading = false;
      $scope.patchesError = resp.err;
    });
  };

  $scope.$watch("filterCommitQueue", function() {
      localStorage.setItem("filterCommitQueue", $scope.filterCommitQueue);
      $scope.loadCurrentPage();
  });

  $rootScope.$on('$locationChangeStart', function() {
    var page = $location.search()['page'];
    if (page) {
      page = parseInt(page, 10);
    } else {
      page = 0;
    }
    if (page !== $scope.currentPage) {
      $scope.currentPage = page;
      $scope.loadCurrentPage();
    }
  });

  $scope.currentPage = $location.search()['page'] || 0;
  $scope.loadCurrentPage();
});
