mciModule.controller('ProjectCtrl', function($scope, $window, $http) {

  $scope.availableTriggers = $window.availableTriggers

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
  $scope.projectVars = {};
  $scope.projectRef = {};
  $scope.displayName = "";

  $scope.projectView = false;

  $scope.settingsFormData = {};
  $scope.saveMessage = "";

  $scope.modalTitle = 'New Project';
  $scope.modalOpen = false;
  $scope.newProject = {};
  $scope.newProjectMessage="";

  $scope.isDirty = false;

  $scope.showProject = function(project) {
    return !(project.length == 0);
  }

  $scope.isBatchTimeValid = function(t){
    if(t==''){
      return true
    }
    return !isNaN(Number(t)) && Number(t) >= 0
  }

  $scope.addAlert = function(obj, trigger){
    if(!$scope.settingsFormData.alert_config) {
      $scope.settingsFormData.alert_config = {}
    } 
    if(!$scope.settingsFormData.alert_config[trigger.id]){
      $scope.settingsFormData.alert_config[trigger.id] = []
    }
    $scope.settingsFormData.alert_config[trigger.id].push(NewEmailAlert(obj.email))
    obj.editing = false
  }

  $scope.getProjectAlertConfig = function(t){
    if(!$scope.settingsFormData.alert_config || !$scope.settingsFormData.alert_config[t]){
      return []
    }
    return $scope.settingsFormData.alert_config[t]
  }

  $scope.findProject = function(identifier){
    return _.find($scope.trackedProjects, function(project){
      return project.identifier == identifier;
    })
  }

  $scope.openAlertModal = function(){
    var modal = $('#alert-modal').modal('show');
  }

  $scope.openAdminModal = function(opt) {
    var modal = $('#admin-modal').modal('show');
    $scope.newProjectMessage = "";
    if (opt === 'newProject') {
      $('#project-name').focus();
      modal.on('shown.bs.modal', function() {
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
          batch_time: parseInt($scope.projectRef.batch_time),
          deactivate_previous: $scope.projectRef.deactivate_previous,
          relative_url: $scope.projectRef.relative_url,
          branch_name: $scope.projectRef.branch_name,
          owner_name: $scope.projectRef.owner_name,
          repo_name: $scope.projectRef.repo_name,
          enabled: $scope.projectRef.enabled,
          alert_config: $scope.projectRef.alert_config || {},
          repotracker_error: $scope.projectRef.repotracker_error || {},
        };

        $scope.displayName = $scope.projectRef.display_name ? $scope.projectRef.display_name : $scope.projectRef.identifier;

      })
      .error(function(data, status) {
        console.log(status);
      });
  };

  $scope.shouldHighlight = function(project) {
    if ($scope.projectRef) {
      return project.identifier == $scope.projectRef.identifier;
    }
    return false;
  }
 
  $scope.saveProject = function() {
    $scope.settingsFormData.batch_time = parseInt($scope.settingsFormData.batch_time)
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
    $scope.isDirty = true;
  };

  $scope.$watch("settingsForm.$dirty", function(dirty) {
    if (dirty){
      $scope.saveMessage = "You have unsaved changes.";
      $scope.isDirty = true;
    } 
  });

  $scope.getAlertDisplay =function(alertObj){
    if(alertObj.provider=='email'){
      return "Send an e-mail to " + alertObj.settings.recipient
    }
    return 'unknown'
  }

  $scope.removeAlert = function(triggerId, index){
    $scope.settingsFormData.alert_config[triggerId].splice(index, 1)
  }

  $scope.setLastRevision = function() {
    if ($scope.settingsFormData.repotracker_error.exists) {
      var revisionUrl = '/project/' + $scope.settingsFormData.identifier + "/repo_revision";
      $http.put(revisionUrl, $scope.settingsFormData.repotracker_error.merge_base_revision).
        success(function(data, status) {
          $scope.settingsFormData.repotracker_error.exists = false;
        }).
        error(function(data, status, errorThrown){
          console.log(status);
        })
    }
  }
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

mciModule.directive('adminNewAlert', function() {
    return {
      restrict: 'E',
      templateUrl:'/static/partials/alert_modal_form.html',
      link: function(scope, element, attrs){
        scope.availableTriggers= [
          {id:"task_failed", display:"Any task fails..."},
          {id:"first_task_failed", display:"The first failure in a version occurs..."},
          {id:"task_fail_transition", display:"A task that had passed in a previous run fails"},
        ]
        scope.availableActions= [
          {id:"email", display:"Send an e-mail"},
        ]
        scope.setTrigger = function(index){
          scope.currentTrigger = scope.availableTriggers[index]
        }
        scope.currentTrigger = scope.availableTriggers[0]
      }
  };
});
