mciModule.controller('VersionMatrixController', function($scope, $window, $location, $filter) {
  $scope.baseVersion = $window.baseVersion;
  $scope.gridCells = $window.gridCells;
  $scope.revisionFailures = $window.revisionFailures;
  $scope.allVersions = $window.allVersions;
  $scope.userTz = $window.userTz;
  $scope.baseRef = $window.baseRef;

  $scope.consts = { recentFailuresView: "RF", 
                    gridView: "GV", 
                    taskFailuresView: "task",
                    testFailuresView: "test",
                    variantFailuresView: "variant", 
                    revisionFailuresView: "revision",
                    numFailures: "NF", 
                    nameSort: "NS",
                    revisionSort: "RS",
                  }

  $scope.taskFailures = [];
  $scope.testFailures = [];
  $scope.variantFailures = [];
  $scope.revisionFailures = [];
  $scope.currentFailureView = $scope.consts.taskFailuresView;
  $scope.sortBy = $scope.consts.numFailures;

  $scope.testHeaders = [
    {name: "Task", by: "task", order: false},
    {name : "Variant", by: "variant", order: false}
  ]

  $scope.taskHeaders = [
    {name:"Test", by: "test", order: false},
    {name : "Variant", by: "variant", order: false}
  ]

  $scope.variantHeaders = [
    {name: "Task", by: "task", order: false},
    {name:"Test", by: "test", order: false}
  ]

  $scope.revisionHeaders = [
    {name: "Task", by: "task", order: false},
    {name:"Test", by: "test", order: false},
    {name: "Variant", by:"variant", order: false},
  ]
  
  $scope.currentHeaders = $scope.taskHeaders;

  $scope.widthPercentage = function(){
    return 96/($scope.currentHeaders.length)
  }

  $scope.selectedHeader = {};

  $scope.setSelectedHeader = function(headerField) {
    if ($scope.selectedHeader.name == headerField.name) {
      $scope.selectedHeader.order = !$scope.selectedHeader.order;
    } else {
      $scope.selectedHeader = headerField;
      $scope.selectedHeader.order = false;
    }
  };

  $scope.selectedClass = function(headerField) {
    var newIcon = 'fa-sort';
    if (headerField.name == $scope.selectedHeader.name) {
      newIcon = 'fa-sort-up';
      if ($scope.selectedHeader.order) {
        newIcon =  'fa-sort-down';
      }
    }
    return newIcon;
  }

  // If a tab name is specified in the URL, parse it out and set the tab
  // name in the scope so that the correct tab is open when the page loads.
  var hash = $location.hash();
  var path = $location.path();

  if (path && (path.substring(1) != "")) {
    $scope.tab = path.substring(1);
  } else if (hash) {
    $scope.tab = hash;
  } else {
    $scope.tab = $scope.consts.recentFailuresView;
  }

  $scope.getTab = function() {
    return $scope.tab;
  }

  $scope.getTab = function() {
    return $scope.tab;
  }

  $scope.setTab = function(tabnum) {
    $scope.tab = tabnum;
    setTimeout(function() {
      $location.hash('' + $scope.tab);
      $scope.$apply();
    }, 0)
  }

  // to be able to toggle hiding and showing individual subtables
  $scope.toggleView = function(index) {
    $scope.currentFailures[index].hidden = !$scope.currentFailures[index].hidden;
  }

  $scope.getToggleClass = function(hidden) {
    return hidden ? "fa-caret-right" : "fa-caret-down";
  }

  // expand all of the current failures
  $scope.expandAll = function() {
    for (var i in $scope.currentFailures) {
      $scope.currentFailures[i].hidden = false;
    }
  }

  // collapse all of the current failures
  $scope.collapseAll = function() {
    for(var i in $scope.currentFailures) {
      $scope.currentFailures[i].hidden = true;
    }
  }

  //variable/function to handle expanding the header message
  $scope.showFullMessage = false;
  $scope.flipExpand = function() {
    $scope.showFullMessage = !$scope.showFullMessage;
  };

  $scope.setSort = function(sort) {
    $scope.sortBy = sort;
    if (sort == $scope.consts.numFailures){
      $scope.sortByFailures();
    } else if( sort == $scope.consts.nameSort) {
      $scope.sortByName();
    } else {
      $scope.sortByRevisionOrder();
    }
  }

  $scope.setSubSort = function(headerField, groupingField, ordering){
    var subGroup = $scope.currentFailures[groupingField];
    subGroup.sort(function(a,b){
      if (a[headerField] < b[headerField]) {
        return 1;
      } else if (a[headerField] > b[headerField]) {
        return -1;
      } else {
        return 0;
      }
    }); 
    $scope.currentFailures[groupingField] = subGroup;
  }

  $scope.getHeaderVal = function(fields, index) {
    return fields[$scope.currentHeaders[index].by];
  }

  // sort by failures takes the current failures and sorts them on the number 
  $scope.sortByFailures = function(){
    $scope.currentFailures.sort(function(a,b){
      if (a.fields.length < b.fields.length) {
        return 1;
      } else if (a.fields.length > b.fields.length) {
        return -1;
      } else {
        return 0;
      }
    })
  }

  // sort by the name of the field
  $scope.sortByName = function(){
    $scope.currentFailures.sort(function(a,b) {
      if (a.groupingField < b.groupingField) {
        return -1;
      } else if (a.groupingField > b.groupingField) {
        return 1 ;
      } else {
        return 0;
      }
    });
  }


  $scope.grid = {};
  $scope.taskNames = [];
  $scope.buildVariants = [];

  $scope.groupByTask = function(){
    // group the failures by task and test
    var failures = {};
    $scope.numTestFailures = 0;
    for (var i = 0; i < $window.failures.length; i++) {
      var failure = $window.failures[i];
      var identifier = failure.identifier;
      identifier.test = $filter('endOfPath')(identifier.test);
      if (!failures[identifier.task]) {
        failures[identifier.task] = [];
        $scope.numTestFailures += 1;
      } 
      for (var j in failure.variants) {
        failures[identifier.task].push({"test": identifier.test, 
                                        "variant": failure.variants[j].name, 
                                        "task_id": failure.variants[j].task_id, 
                                         });
      }
    }
    // sort failures by number of failing tests
    $scope.taskFailures = [];
    _.each(failures,function(value, key) {
      $scope.taskFailures.push({
        "groupingField": key,
        "fields": value, 
        "hidden" : false,
      });
    });
  }

  $scope.groupByTest = function(){
    // group the failures by task and test
    var failures = {};
    $scope.numTestFailures = 0;
    for (var i = 0; i < $window.failures.length; i++) {
      var failure = $window.failures[i];
      var identifier = failure.identifier;
      identifier.test = $filter('endOfPath')(identifier.test);
      if (!failures[identifier.test]) {
        failures[identifier.test] = [];
        $scope.numTestFailures += 1;
      } 
      for (var j in failure.variants) {
        failures[identifier.test].push({"task": identifier.task, 
                                        "variant": failure.variants[j].name, 
                                        "task_id": failure.variants[j].task_id, 
                                        });
      }
    }
    _.each(failures,function(value, key) {
      $scope.testFailures.push({
        "groupingField": key,
        "fields": value, 
        "hidden" : false,
      });
    });
  }

  $scope.groupByVariant = function(){
    // group the failures by task and test
    var failures = {};
    $scope.numTestFailures = 0;
    for (var i = 0; i < $window.failures.length; i++) {
      var failure = $window.failures[i];
      var identifier = failure.identifier;
      identifier.test = $filter('endOfPath')(identifier.test);
      for (var j in failure.variants) {
        if (!failures[failure.variants[j].name]) {
          failures[failure.variants[j].name] = [];
        }
        failures[failure.variants[j].name].push({"task": identifier.task, 
                                                "test": identifier.test, 
                                                "task_id": failure.variants[j].task_id, 
                                                });
      }
    }
    _.each(failures,function(value, key) {
      $scope.variantFailures.push({
        "groupingField": key,
        "fields": value, 
        "hidden" : false,
      });
    });
  }

  $scope.groupByRevision = function(){
    // group the revision failures by task, test and variant
    var failures = {};
    $scope.numTestFailures = 0;
    for (var i = 0; i < $window.revisionFailures.length; i++) {
      var identifier = $window.revisionFailures[i].revision;
      var revFailures = $window.revisionFailures[i].failures;
      for (j in revFailures){
        var failure = revFailures[j];
        failure.test = $filter('endOfPath')(failure.test);
        if (!failures[identifier]) {
          failures[identifier] = [];
        }
        failures[identifier].push({ "task": failure.task, 
                                    "test": failure.test, 
                                    "task_id": failure.task_id, 
                                    "variant": failure.variant,
                                  });
      
    }
}
    _.each(failures,function(value, key) {
      $scope.revisionFailures.push({
        "groupingField": key,
        "order": $scope.allVersions[key].order, // add the order field from the dictionary of all versions. 
        "fields": value, 
        "hidden" : false,
      });
    });

    // sort on revision order
    $scope.revisionFailures.sort(function(a,b) {
      if (a.order < b.order) {
        return 1;
      } else if (a.order > b.order) {
        return -1;
      } else {
        return 0;
      }
    });

  }


  // creates the grid view
  $scope.createGrid = function () {
      // create grid with map of buildvariant to its tasks
  for (var i = 0; i < gridCells.length; i++) {
    var task = gridCells[i].cellId.task;
    var variant = gridCells[i].cellId.variant;
    if (!$scope.grid[variant]) {
      $scope.grid[variant] = {};
      $scope.buildVariants.push(variant);
    }
    if (!$scope.grid[variant][task]) {
      $scope.grid[variant][task] = {
        "current": gridCells[i].history[0],
      };
      if ($scope.taskNames.indexOf(task) == -1)
        $scope.taskNames.push(task);
      $scope.grid[variant][task].prevTasks = gridCells[i].history.slice(1);
      $scope.grid[variant][task].prevStatus = cellStatus(gridCells[i].history.slice(1));
    }
  }

  // sort tasks and buildvariants alphabetically
  $scope.taskNames.sort();
  $scope.buildVariants.sort();

  $scope.currentTask = null;
  $scope.currentCell = '';
  $scope.currentBuildVariant = '';
  $scope.currentTaskName = '';
  }


  // pre-create all the groupings
  $scope.groupByTask();
  $scope.groupByTest();
  $scope.groupByVariant();
  $scope.groupByRevision();

  $scope.createGrid();

  // set the default current failures to task failures
  $scope.currentFailures = $scope.taskFailures;
  $scope.setSort($scope.sortBy);


  $scope.setFailureView = function(view) {
    $scope.currentFailureView = view;
    if (view == $scope.consts.taskFailuresView) {
      $scope.currentFailures = $scope.taskFailures;
      $scope.currentHeaders = $scope.taskHeaders;
    } else if (view == $scope.consts.testFailuresView) {
      $scope.currentFailures = $scope.testFailures;
      $scope.currentHeaders = $scope.testHeaders;
    } else if (view == $scope.consts.variantFailuresView) {
      $scope.currentFailures = $scope.variantFailures;
      $scope.currentHeaders = $scope.variantHeaders;
    } else {
      $scope.currentFailures = $scope.revisionFailures;
      $scope.currentHeaders = $scope.revisionHeaders;
    }
    if (view != $scope.consts.revisionFailuresView){
      $scope.setSort($scope.sortBy);
    }
  }

  $scope.getRevisionMessage = function(revision) {
    return $scope.allVersions[revision].message;
  };

  $scope.showTaskPopover = function(buildVariant, task, target) {
    $scope.currentTask = target;
    if ($scope.grid[buildVariant] && $scope.grid[buildVariant][task]) {
      $scope.currentCell = $scope.grid[buildVariant][task];
      $scope.currentBuildVariant = buildVariant;
      $scope.currentTaskName = task;
    } else {
      $scope.currentCell = null;
    }
  };

  function cellStatus(history) {
    for (var i = 0; i < history.length; i++) {
      if (history[i].status == 'success') {
        return history[i].status;
      } else if (history[i].status == 'failed') {
        if ('task_end_details' in history[i]) {
          if ('type' in history[i].task_end_details) {
            if (history[i].task_end_details.type == 'system') {
              return 'system-failed';
            }
          }
        }
        return 'failure';
      }
    }
    return 'undispatched';
  }

  $scope.highlightHeader = function(row, col) {
    $('.header-cell.highlighted').removeClass('highlighted');
    $($('.header-cell').get(col)).addClass('highlighted');
    $('.tablerow .header').removeClass('highlighted');
    $($('.tablerow .header').get(row)).addClass('highlighted');
  };

  $scope.getGridClass = function(variant, task) {
    var cellClass = '';
    if (!$scope.grid[variant])
      return 'skipped';
    var cell = $scope.grid[variant][task];
    if (!cell) return 'skipped';
    if (cell.current) {
      if (cell.current.status == 'undispatched') {
        cellClass = 'was-' + cell.prevStatus;
      } else if (cell.current.status == 'failed') {
        cellClass = 'failure';
        if ('task_end_details' in cell.current) {
          if ('type' in cell.current.task_end_details) {
            if (cell.current.task_end_details.type == 'system') {
              cellClass = 'system-failed';
            }
          }
        }
      } else if (cell.current.status == 'success') {
        cellClass = 'success';
      } else if (cell.current.status == 'started' || cell.current.status == 'dispatched') {
        cellClass = 'was-' + cell.prevStatus + ' started';
      }
      return cellClass;
    } else {
      return "was-" + cell.prevStatus;
    }
  };
});
