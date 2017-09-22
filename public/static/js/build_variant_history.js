mciModule.controller('BuildVariantHistoryController',['$scope', '$filter','$window', 'mciBuildVariantHistoryRestService', 'notificationService', function($scope, $filter, $window,
  mciBuildVariantHistoryRestService, notifier) {
  $scope.buildVariant = $window.buildVariant;
  $scope.project = $window.project;
  $scope.taskNames = $window.taskNames;
  $scope.versionsByRevision = {};
  $scope.versions = [];
  $scope.tasksByTaskNameByCommit = [];
  $scope.lastVersionId = '';

  var buildVersionsByRevisionMap = function(versions) {
    _.each(versions, function(version) {
      $scope.versions.push(version);
      $scope.versionsByRevision[version.revision] = version;
    });
  };

  var buildTasksByTaskNameCommitMap = function(tasksByCommit) {
    _.each(tasksByCommit, function(revisionToTasks) {
      var taskNameTaskMap = {};
      _.each(revisionToTasks.tasks, function(task) {
        taskNameTaskMap[task.display_name] = task;
      });

      $scope.tasksByTaskNameByCommit.push({
        _id: revisionToTasks._id,
        tasksByTaskName: taskNameTaskMap
      });
    });
  };

  /* Populate initial page data */
  buildVersionsByRevisionMap($window.versions);
  buildTasksByTaskNameCommitMap($window.tasksByCommit);

  var numVersions = $scope.versions.length;
  if (numVersions > 0) {
    $scope.lastVersionId = $scope.versions[numVersions - 1].id;
  }

  $scope.getGridClass = function(cell) {
    if (cell) {
      return $filter('statusFilter')(cell);
    }

    return "skipped";
  };

  $scope.loadMore = function() {
    mciBuildVariantHistoryRestService.getBuildVariantHistory(
      $scope.project,
      $scope.buildVariant, {
        format: 'json',
        before: $scope.lastVersionId,
      }, {
        success: function(resp) {
          var data = resp.data;
          if (data.Versions) {
            buildVersionsByRevisionMap(data.Versions);
            $scope.lastVersionId = $scope.versions[$scope.versions.length - 1].id;
          }

          if (data.Tasks) {
            buildTasksByTaskNameCommitMap(data.Tasks);
          }
        },

        error: function(resp) {
          notifier.pushNotification('Error getting build variant history: ' + resp.data.error,'errorHeader');
        }
      }
    );
  };
}]);
