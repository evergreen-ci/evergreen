mciModule.controller('PatchController', function($scope, $filter, $window, notificationService, $http) {
  $scope.userTz = $window.userTz;
  $scope.canEdit = $window.canEdit;
  $scope.enabledTasks = _.pluck($window.tasks, "Name");
  $scope.disableSubmit = false;
  if (window.hasBanner && !isDismissed(bannerText())) {
    $("#drawer").addClass("bannerMargin");
    $("#content").addClass("bannerMargin");
  }

  var checkedProp = _.property("checked");
  var variant = "";

  // Event handler for when the user clicks on one of the variants
  // in the left panel. Also accounts for special toggle behavior when the user
  // is holding shift/meta/ctrl when clicking.
  $scope.selectVariant = function($event, index){
    variant = $scope.variants[index];
    $event.preventDefault();
    if ($event.ctrlKey || $event.metaKey) {
      // Ctrl/Meta+Click: Toggle just the variant being clicked.
      $scope.variants[index].checked = !$scope.variants[index].checked;
    } else if ($event.shiftKey) {
      // Shift+Click: Select everything between the first element
      // that's already selected element and the element being clicked on.
      var firstCheckedIndex = _.findIndex($scope.variants, checkedProp);
      firstCheckedIndex = Math.max(firstCheckedIndex, 0); // if nothing selected yet, start at 0.
      var indexBounds = Array(firstCheckedIndex, index).sort(function(a, b){
        return a-b;
      })
      for(var i=indexBounds[0]; i<=indexBounds[1]; i++){
        $scope.variants[i].checked = true;
      }
    } else {
      // Regular click: Select *only* the one being clicked, and unselect all others.
      for(var i=0; i<$scope.variants.length;i++){
        $scope.variants[i].checked = (i == index);
      }
    }
  }

  // Returns an object containing the total number of tasks to be scheduled,
  // and the number of variants with at least 1 task to be scheduled.
  // Return value is in the format format {numVariants: 3, numTasks: 25}
  $scope.selectionCount = function(){
    var numVariants = _.filter($scope.variants, function(x){return _.filter(x.tasks, checkedProp).length > 0}).length;
    var numTasks = _.reduce(_.map($scope.variants, function(x){return _.filter(x.tasks, checkedProp).length}), function(x, y){return x+y}, 0);
    return {numVariants: numVariants, numTasks: numTasks};
  }

  // Returns the number of tasks checked only within the specified variant.
  // Used to populate the count badge next to each variant in the left panel
  $scope.numSetForVariant = function(variantId){
    var v = _.find($scope.variants, function(x){return x.id == variantId});
    return _.filter(_.pluck(v.tasks, "checked"), _.identity).length;
  }

  // Gets the subset of variants that are active by the user
  $scope.selectedVariants = function(){
    return _.filter($scope.variants, checkedProp);
  }

  $scope.isMac = function(){
    return navigator.platform.toUpperCase().indexOf('MAC')>=0;
  }

  // Gets the list of tasks that are active across all the list of currently
  // selected variants, sorted by name. Used to populate the field of
  // checkboxes in the main panel.
  $scope.getActiveTasks = function(){
    var selectedVariants = $scope.selectedVariants();

    // return the union of the set of tasks shared by all of them, sorted by name
    var tasksInSelectedVariants = _.uniq(_.flatten(_.map(_.pluck(selectedVariants, "tasks"), _.keys)));
    return tasksInSelectedVariants.sort();
  }

  // Click handler for "all" and "none" links. Applies the given state to all
  // tasks within the current set of active variants
  $scope.changeStateAll = function(state){
    var selectedVariantNames = _.object(_.map(_.pluck($scope.selectedVariants(), "id"), function(id){return [id, true]}));
    var activeTasks = $scope.getActiveTasks();
    for(var i=0;i<$scope.variants.length;i++){
      var v = $scope.variants[i];
      if(!(v.id in selectedVariantNames)){
        continue;
      }
      _.each(activeTasks, function(taskName){
        if(_.has(v.tasks, taskName)){
          v.tasks[taskName].checked = state;
        }
      })
    }
  }

  $scope.isUnauthorizedPRPatch = (patch.Version.length === 0 && patch.GithubPatchData.PRNumber !== 0);
  $scope.isPRPatch = patch.GithubPatchData.PRNumber !== 0;

  function collectVariantTasks() {
      return _.filter(_.map($scope.variants, function(v){
          var tasks = [];
          var displayTasks = [];
          for (var name in v.tasks) {
              if (v.tasks[name].checked) {
                  if (v.tasks[name].displayOnly) {
                      displayTasks.push({ "name": name, "execTasks": v.tasks[name].execTasks});
                  }
                  else {
                      tasks.push(name);
                  }
              }
          }
          return {
              variant: v.id,
              tasks: tasks,
              displayTasks: displayTasks
          };
      }), function(v){return (v.tasks.length > 0) || (v.displayTasks.length > 0)})
  }

  // Sends the current patch config to the server to save.
  $scope.save = function(){
    $scope.disableSubmit = true;
    var data = {
      "description": $scope.patch.Description,
      "variants_tasks": collectVariantTasks()
    }
    $http.post('/patch/' + $scope.patch.Id, data).then(
      function(resp) {
        window.location.replace("/version/" + resp.data.version);
      },
      function(resp) {
      	notificationService.pushNotification('Error retrieving logs: ' + JSON.stringify(resp.data), 'errorHeader');
      });
  };

  $scope.setPatchInfo = function() {
    $scope.patch = $window.patch;
    $scope.patchContainer = {'Patch':$scope.patch};
    var patch = $scope.patch;
    var variantsFilteredTasks = _.mapObject($window.variants, function(v, k){
      v.Tasks = _.filter(v.Tasks, function(x){return _.contains($scope.enabledTasks, x.Name)});
      return v;
    })
    $scope.variants = _.sortBy(_.map(variantsFilteredTasks, function(v, variantId){
      var tasks = {};
      var execTasks = {};
      if (v.DisplayTasks && v.DisplayTasks.length > 0) {
        v.DisplayTasks.forEach(function(task) {
          tasks[task.Name] = {checked: false, displayOnly: true, execTasks: []};
          task.ExecutionTasks.forEach(function(execTask) {
            execTasks[execTask] = "";
            tasks[task.Name].execTasks.push(execTask);
          });
        });
      }
      v.Tasks.forEach(function(task) {
        if (!execTasks.hasOwnProperty(task.Name)) {
          tasks[task.Name] = {checked: false, displayOnly: false};
        }
      });
      return {
        id: variantId,
        checked:false,
        name: v.DisplayName,
        tasks: tasks
      };
    }), "name");

    // If there's only one variant, just pre-select it.
    if($scope.variants.length == 1 ){
      $scope.variants[0].checked = true;
    }

    var allUniqueTaskNames = _.uniq(_.flatten(_.map(_.pluck($scope.variants, "tasks"), _.keys)));

    $scope.tasks = _.object(_.map(allUniqueTaskNames, function(taskName){
      // create a getter/setter for the state of the task
      return [taskName, function(newValue){
        var selectedVariants = $scope.selectedVariants();
        if(!arguments.length){ // called with no args, act as a getter
          var statusAcrossVariants = _.flatten(_.map(_.pluck($scope.selectedVariants(), "tasks"), function(o){return _.filter(o, function(v, k){return k==taskName})}));
          var groupCountedStatus = _.countBy(statusAcrossVariants, function(x){return x.checked == true});
          if(groupCountedStatus[true] == statusAcrossVariants.length ){
            return true;
          }else if(groupCountedStatus[false] == statusAcrossVariants.length ){
            return false;
          }
          return null;
        }

        var selectedVariantNames = _.object(_.map(_.pluck(selectedVariants, "id"), function(id){return [id, true]}));

        // act as a setter
        for(var i=0;i<$scope.variants.length;i++){
          var v = $scope.variants[i];
          if(!(v.id in selectedVariantNames)){
            continue;
          }
          if(_.has(v.tasks, taskName)){
            v.tasks[taskName].checked = newValue;
          }
        }
        return newValue;
      }];
    }))
  }

  // Older patches may only have the fields "Variants" and "Tasks" but newer patches
  // have a VariantsTasks field that has all pairs grouped together by variant.
  // This function backfills the VariantsTasks field for older patches that were created
  // before the schema change.
  if(!patch.VariantsTasks && (patch.Tasks || []).length > 0 && (patch.BuildVariants || []).length > 0){
    patch.VariantsTasks = _.map(patch.BuildVariants, function(v){
      // The _intersection limits the set of tasks to be included
      // to just the ones that actually exist in the config for that variant.
      return {Variant:v, Tasks: _.intersection(_.pluck(($window.variants[v] || {}).Tasks , "Name"), patch.Tasks)};
    });
  }

  $scope.setPatchInfo();

  var updateExistingTasks = function(task) {
    if (task) {
      task.checked = true;
      if(!!patch.Version){
        // if the task was already created, we can't uncheck the box
        task.disabled = true;
      }
    }
  }

  // Populate the checkboxes in the UI according to the variants and tasks that
  // were specified on the command line, or that may have already been created
  // if the patch was finalized already.
  if((patch.VariantsTasks || []).length>0){
    for(var i=0;i<patch.VariantsTasks.length;i++){
      var vt = patch.VariantsTasks[i];
      var variantIndex = _.findIndex($scope.variants, function(x){return x.id == patch.VariantsTasks[i].Variant});
      if(variantIndex >= 0 ){
        _.each(vt.Tasks, function(x){
          var task = $scope.variants[variantIndex].tasks[x];
          updateExistingTasks(task);
        })
        _.each(vt.DisplayTasks, function(x){
          var dt = $scope.variants[variantIndex].tasks[x.Name];
          updateExistingTasks(dt);
        })
      }
    }
  }

  $scope.defaultVTs = collectVariantTasks();

  // When authorizing github patches, do not allow the authorizer to
  // disable the default set of tasks
  $scope.checkDisabledState = function(task) {
    if (!$scope.isUnauthorizedPRPatch) {
        return false;
    }
    for (var i = 0; i < $scope.defaultVTs.length; i++) {
        if (variant.id !== $scope.defaultVTs[i].variant) {
            continue
        }
        if ($scope.defaultVTs[i].tasks.includes(task)) {
            return true;
        }
    }

    return false
  }
})
