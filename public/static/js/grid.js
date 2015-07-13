mciModule.controller('VersionMatrixController', function($scope, $window, $location, $filter) {
  $scope.baseVersion = $window.baseVersion;
  $scope.gridCells = $window.gridCells;
  $scope.allVersions = $window.allVersions;
  $scope.userTz = $window.userTz;
  $scope.baseRef = $window.baseRef;

  // If a tab number is specified in the URL, parse it out and set the tab
  // number in the scope so that the correct tab is open when the page loads.
  var hash = $location.hash();
  var path = $location.path();
  if (path && !isNaN(parseInt(path.substring(1)))) {
    $scope.tab = parseInt(path.substring(1));
  } else if (!isNaN(parseInt(hash))) {
    $scope.tab = parseInt(hash);
  } else {
    $scope.tab = 1;
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

  $scope.grid = {};
  $scope.taskNames = [];
  $scope.buildVariants = [];

  // group the failures by task and test
  var failures = {};
  $scope.numTestFailures = 0;
  for (var i = 0; i < $window.failures.length; i++) {
    var failure = $window.failures[i];
    var identifier = failure.identifier;
    identifier.test = $filter('endOfPath')(identifier.test);
    if (!failures[identifier.task]) {
      failures[identifier.task] = {};
      failures[identifier.task][identifier.test] = [].concat(failure.variants);
      $scope.numTestFailures += 1;
    } else {
      if (!failures[identifier.task][identifier.test]) {
        failures[identifier.task][identifier.test] = [];
      }
      var variants = failures[identifier.task][identifier.test];
      failures[identifier.task][identifier.test] = variants.concat(failure.variants);
      $scope.numTestFailures += 1;
    }
  }

  // sort failures by number of failing tests
  $scope.failures = [];
  Object.keys(failures).forEach(function(t) {
    $scope.failures.push({
      "task": t,
      "variants": failures[t]
    });
  });

  $scope.failures.sort(function(a, b) {
    var numA = 0;
    var numB = 0;
    Object.keys(a.variants).forEach(function(v) {
      numA += a.variants[v].length;
    });
    Object.keys(b.variants).forEach(function(v) {
      numB += b.variants[v].length;
    });
    if (numB == numA)
      return a.task > b.task;
    return numB > numA;
  });

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