function PatchController($scope, $filter, $window, notificationService) {
  $scope.userTz = $window.userTz;

  $scope.selectedTask = {
    v: "",
    get: function() {
      return $scope.selectedTask.v;
    },
    set: function(val) {
      $scope.selectedTask.v = val;
    }
  };

  $scope.selectedVariant = {
    v: "",
    get: function() {
      return $scope.selectedVariant.v;
    },
    set: function(val) {
      $scope.selectedVariant.v = val;
    }
  };

  $scope.setPatchInfo = function() {
    $scope.patch = $window.patch;
    var patch = $scope.patch;
    var allTasks = _.sortBy($window.tasks, 'Name')
    var allVariants = $window.variants;

    var allVariantsModels = [];
    var allVariantsModelsOriginal = [];
    for (var variantId in allVariants) {
      var variant = {
        "name": allVariants[variantId].DisplayName,
        "id": variantId,
        "tasks": _.map(allVariants[variantId].Tasks, function(task) {
          return task.Name;
        })
      };
      if ($.inArray(variant.id, patch.BuildVariants) >= 0)  {
        variant.checked = true;
      }
      allVariantsModels.push(variant);
      allVariantsModelsOriginal.push(_.clone(variant));
    }

    // Create a map from tasks to list of build variants that run that task
    $scope.buildVariantsForTask = {};
    _.each(allVariantsModelsOriginal, function(variant) {
      _.each(variant.tasks, function(task) {
        $scope.buildVariantsForTask[task] =
          $scope.buildVariantsForTask[task] || [];
        if (variant.id &&
          $scope.buildVariantsForTask[task].indexOf(variant.id) == -1) {
          $scope.buildVariantsForTask[task].push(variant.id);
        }
      });
    });

    var allTasksModels = [];
    var allTasksModelsOriginal = [];
    for (var i = 0; i < allTasks.length; ++i) {
      task = allTasks[i];
      if (task.Name === "compile" || $.inArray(task.Name, patch.Tasks) >= 0) {
        task.checked = true;
      }
      allTasksModels.push(task);
      allTasksModelsOriginal.push(_.clone(task));
    }
    $scope.allTasks = allTasksModels;
    $scope.allTasksOriginal = allTasksModelsOriginal;
    $scope.allVariants = allVariantsModels;
    $scope.allVariantsOriginal = allVariantsModelsOriginal;

    $scope.$watch('allVariants', function(allVariants) {
      $scope.variantsCount = 0;
      _.forEach(allVariants, function(item) {
        if (item.checked) {
          $scope.variantsCount += 1;
        }
      });
    }, true);

    $scope.$watch('allTasks', function(allTasks) {
      $scope.taskCount = 0;
      _.forEach(allTasks, function(item) {
        if (item.checked) {
          $scope.taskCount += 1;
        }
      });
    }, true);
  };
  $scope.setPatchInfo();

  $scope.selectedVariants = function() {
    return $filter('filter')($scope.allVariants, {
      checked: true
    });
  };

  $scope.selectedTasks = function() {
    return $filter('filter')($scope.allTasks, {
      checked: true
    });
  };

  $scope.toggleCheck = function(x) {
    x.checked = !x.checked;
  };

  $scope.variantRunsTask = function(variant, taskName) {
    // Does this variant run the given task name?
    return variant.tasks.indexOf(taskName) != -1;
  };

  $scope.taskRunsOnVariant = function(taskName, variant) {
    // Does this task name run on the variant with the given name?
    return ($scope.buildVariantsForTask[taskName] || []).indexOf(variant) != -1;
  };
}

function PatchUpdateController($scope, $http) {
  $scope.scheduleBuilds = function(form) {
    var data = {
      variants: [],
      tasks: [],
      description: $scope.patch.Description
    };
    var selectedVariants = $scope.selectedVariants();
    var selectedTasks = $scope.selectedTasks();

    for (var i = 0; i < selectedVariants.length; ++i) {
      data.variants.push(selectedVariants[i].id);
    }

    for (var i = 0; i < selectedTasks.length; ++i) {
      data.tasks.push(selectedTasks[i].Name);
    }

    $http.post('/patch/' + $scope.patch.Id, data).
    success(function(data, status) {
      window.location.replace("/version/" + data.version);
    }).
    error(function(data, status, errorThrown) {
      alert("Failed to save changes: `" + data + "`",'errorHeader');
    });
  };

  $scope.select = function(models, state) {
    for (var i = 0; i < models.length; ++i) {
      models[i].checked = state;
    }
  };
}
