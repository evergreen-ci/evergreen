/**
 *  version.js
 *
 *  Created on: September 4, 2013
 *      Author: Valeri Karpov <valeri.karpov@mongodb.com>
 *
 *  AngularJS controllers for /ui/templates/version.html
 *
 */

function VersionController($scope, $http, $now, $window) {
  $scope.version = {};
  $scope.taskStatuses = {};

  $scope.setVersion = function(version) {
    $scope.version = version;

    $scope.commit = {
      message : $scope.version.Version.message,
      author : $scope.version.Version.author,
      author_email : $scope.version.Version.author_email,
      push_time : $scope.version.Version.create_time,
      gitspec : $scope.version.Version.revision,
      repo_owner : $scope.version.repo_owner,
      repo_name : $scope.version.repo_name
    };

    $scope.taskStatuses = {};
    for (var i = 0; i < version.Builds.length; ++i) {
      $scope.taskStatuses[version.Builds[i].Build._id] = [];
      for (var j = 0; j < version.Builds[i].Tasks.length; ++j) {
        $scope.taskStatuses[version.Builds[i].Build._id].push({
          "class" : version.Builds[i].Tasks[j].Task.status,
          "tooltip" : version.Builds[i].Tasks[j].Task.display_name + " - " + version.Builds[i].Tasks[j].Task.status,
          "link" : "/task/" + version.Builds[i].Tasks[j].Task.id
        });
      }
    }
    $scope.lastUpdate = $now.now();
  };

  $scope.load = function() {
    $http.get('/version_json/' + $scope.version.Version.id).
        success(function(data) {
          if (data.error) {
            alert(data.error);
          } else {
            $scope.setVersion(data);
          }
        }).
        error(function(data) {
          alert("Error occurred - " + data);
        });
  };

  $scope.setVersion($window.version);
	$scope.plugins = $window.plugins
}

function VersionHistoryController($scope, $http, $now) {
  $scope.versions = [];
  $scope.buildStatuses = {};

  $scope.setVersions = function(versions) {
    $scope.versions = versions;

    $scope.buildStatuses = {};
    for (var i = 0; i < versions.length; ++i) {
      $scope.buildStatuses[versions[i].Version.id] = [];
      for (var j = 0; j < versions[i].Builds.length; ++j) {
        $scope.buildStatuses[versions[i].Version.id].push({
          "class" : versions[i].Builds[j].Build.status,
          "tooltip" : versions[i].Builds[j].Build.display_name + " - " + versions[i].Builds[j].Build.status,
          "link" : '/build/' + versions[i].Builds[j].Build._id
        });
      }
    }

    $scope.lastUpdate = $now.now();
  };

  $scope.load = function(version) {
    $http.get('/json/version_history/' + version.Version.id).
        success(function(data) {
          if (data.error) {
            alert(data.error);
          } else {
            $scope.setVersions(data);
          }
        }).
        error(function(data) {
          alert("Error occurred - " + data);
        });
  };
}
