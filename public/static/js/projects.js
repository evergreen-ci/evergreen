mciModule.controller('ProjectCtrl', function($scope, $window, $http) {

  $scope.refreshTrackedProjects = function(trackedProjects) { 
    $scope.trackedProjects = trackedProjects
    $scope.enabledProjects = _.filter($scope.trackedProjects, function(p){
      return p.enabled;
    });
    $scope.disabledProjects = _.filter($scope.trackedProjects, function(p) {
      return !(p.enabled);
    });    
  };

  $scope.refreshTrackedProjects($window.allTrackedProjects);
  $scope.projectVars = $window.projectVars;
  $scope.projectRef = $window.projectRef;

  $scope.projectView = false;

  $scope.settingsFormData = {};
  $scope.saveMessage = "";

  $scope.modalTitle = 'New Project';
  $scope.modalOpen = false;
  $scope.adminOptionVals = {};
  $scope.newProject = {};
  $scope.newProjectMessage="";

  $scope.isDirty = false;

  $scope.findProject = function(identifier){
    return _.find($scope.trackedProjects, function(project){
      return project.identifier == identifier;
    })
  }

  $scope.openAdminModal = function(opt) {
    var modal = $('#admin-modal').modal('show');
    $scope.newProjectMessage = "";
    if (opt === 'newProject') {
      modal.on('shown.bs.modal', function() {
          $('#project-name').focus();
          $scope.modalOpen = true;
      });

      modal.on('hide.bs.modal', function() {
        $scope.modalOpen = false;
      });
    }
  };

  $scope.addProject = function() {
    $scope.modalOpen = false;
    $('#admin-modal').modal('hide');
    
    if ($scope.findProject($scope.newProject.identifier)) {
      console.log("project identifier already exists");
      $scope.newProjectMessage = "Project name already exists.";
      $scope.newProject = {};
      return;
    }
    $http.put('/project/' + $scope.newProject.identifier, $scope.newProject)
    .success(function(data, status) {
      $scope.refreshTrackedProjects(data.AllProjects);
      $scope.loadProject(data.ProjectId);
      $scope.newProject = {};
    })
    .error(function(data, status){
      console.log(status);
    });

  };

  $scope.loadProject = function(projectId) {
    $http.get('/project/' + projectId)
      .success(function(data, status){

        $scope.projectView = true;
        $scope.projectRef = data.ProjectRef;

        if (data.ProjectVars) {
         $scope.projectVars = data.ProjectVars.vars;
        }
        else {
          $scope.projectVars = {};
        }

        $scope.settingsFormData = {
          identifier : $scope.projectRef.identifier,   
          project_vars: $scope.projectVars,
          display_name : $scope.projectRef.display_name,
          remote_path:$scope.projectRef.remote_path,
          batch_time: $scope.projectRef.batch_time,
          relative_url: $scope.projectRef.relative_url,
          branch_name: $scope.projectRef.branch_name,
          owner_name: $scope.projectRef.owner_name,
          repo_name: $scope.projectRef.repo_name,
          enabled: $scope.projectRef.enabled,
        };
      })
      .error(function(data, status) {
        alert("Error loading project");
        console.log(status);
      });
  };
 
  $scope.saveProject = function() {
    $http.post('/project/' + $scope.settingsFormData.identifier, $scope.settingsFormData).
      success(function(data, status) {
        $scope.saveMessage = "Settings Saved.";
        $scope.refreshTrackedProjects(data.AllProjects);
        $scope.settingsForm.$setPristine();
        $scope.isDirty = false;
      }).
      error(function(data, status, errorThrown) {
        console.log(status);
      });
  };

  $scope.addProjectVar = function() {
    if ($scope.proj_var.name && $scope.proj_var.value) {
      $scope.settingsFormData.project_vars[$scope.proj_var.name] = $scope.proj_var.value;
      $scope.proj_var.name="";
      $scope.proj_var.value="";
    }
  };

  $scope.removeProjectVar = function(name) {
    delete $scope.settingsFormData.project_vars[name];
  };

  $scope.$watch("settingsForm.$dirty", function(dirty) {
    if (dirty){
      $scope.saveMessage = "Click button to save settings.";
      $scope.isDirty = true;
    } 
  });
});

mciModule.directive('adminNewProject', function() {
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Enter project name ' +
        '<form style="display: inline" ng-submit="addProject()">' +
          '<input type="text" id="project-name" placeholder="project name" ng-model="newProject.identifier">' +
        '</form>' +
        '<button type="submit" class="btn btn-primary" style="float: right; margin-left: 10px;" ng-click="addProject()">Create Project</button>' +
      '</div>' +
    '</div>'
  };
});
