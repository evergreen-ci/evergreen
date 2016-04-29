/**
 *  project_controller.js
 *
 *  Created on: March 10, 2014
 *      Author: Valeri Karpov
 *
 *  Common controller which handles tracking the current project on the
 *  client side.
 *
 */

mciModule.controller('ProjectController', function($scope, $window) {
  $scope.project = $window.project;
  $scope.allProjects = $window.allProjects;
  $scope.projectName = $window.projectName;
  if ($window.appPlugins){
    $scope.appPlugins = $window.appPlugins;
  }

  // groupedProject is an array of arrays representing key-value pairs,
  // where the key is a github repository, and the value is a list
  // of projects based on the repository. Ex.
  //   [["mongo/mongodb", [{},{},{}]], ["10gen/scout", [{}]], ... ]
  $scope.groupedProjects = _.pairs(_.groupBy($scope.allProjects, function(p){
    return [p.owner_name, p.repo_name].join('/');
  }));
  $scope.getName = function(project) {
    return project.display_name ? project.display_name : project.identifier;
  };
  $scope.getGroupName = function(kvPair) {
    // if there are multiple entries, return the repo name
    if (kvPair[1].length > 1) {
      return kvPair[0];
    }
    // otherwise return the project name
    return $scope.getName(kvPair[1][0]);
  };

  $scope.$on('loadProject', function(event, project, projectName) {
    $scope.project = project;
    $scope.projectName = projectName;
  });
});
