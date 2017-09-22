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
    $scope.loading = true;
    $scope.uiPatches = [];
    $scope.patchesError = null;

    var params = {
      params: {
        page: $scope.currentPage
      }
    };
    $http.get(endpoint, params).then(
    function(resp) {
      var data = resp.data;
      $scope.loading = false;
      $scope.versionsMap = data['VersionsMap'];
      $scope.uiPatches = data['UIPatches'];

      _.each($scope.uiPatches, function(patch) {
          patch.canEdit = ($window.user.Id === patch.Patch.Author ) || $window.isSuperUser
      });

      _.each($scope.versionsMap, function(version) {
        _.each(version.Builds, function(build) {
          build.taskResults = [];
          _.each(build.Tasks, function(task) {
            build.taskResults.push({
              link: '/task/' + task.Task.id,
              tooltip: task.Task.display_name,
              'class': $filter('statusFilter')(task.Task),
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
