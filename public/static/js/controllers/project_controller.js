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

function ProjectController($scope, $location, $window) {
  $scope.project = $window.project
  $scope.allProjects = $window.allProjects
  $scope.projectName = $window.projectName
  $scope.groupedProjects = _.groupBy($scope.allProjects, function(p){
    return [p.owner_name, p.repo_name].join('/')
  })
}
