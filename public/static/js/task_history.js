mciModule.factory('taskHistoryFilter', function($http, $window, $filter) {
  var ret = {};

  if (window.hasBanner && !isDismissed(bannerText())) {
    $("#content").addClass("bannerMargin");
    $("#filters").addClass("bannerMargin");
  }

  /* Getter/setter wrapper around the URL hash */
  ret.locationHash = {
    get: function() {
      var hash = $window.location.hash.substr(1); // Get rid of leading '#'
      if (hash.charAt(0) == '/') {
        hash = hash.substr(1);
      }

      return hash;
    },
    set: function(v) {
      $window.location.hash = v;
    }
  };

  /* Convert `ret.filter` to a readable string and back for use in the location
  * hash. */
  var filterSerializer = {
    // Converts `ret.filter` into a readable string
    serialize: function() {
      /* buildVariants is a single string delimited by ',' and each buildVariant
      * gets piped through encodeURIComponent */
      return 'buildVariants=' +
      _.map(ret.filter.buildVariants, encodeURIComponent).join(',');
    },
    /* The inverse of `serialize`. Takes a string, parses it, and sets
    * `ret.filter` to the parsed value. */
    deserialize: function(str) {
      ret.filter = ret.filter || {};
      ret.filter.buildVariants = [];

      var buildVariantsArr = str.split('=');
      if (buildVariantsArr.length > 1) {
        /* If buildVariants isn't empty, split buildVariants by ',' delimiter
        * and do a URI decode on each of them */
        ret.filter.buildVariants = buildVariantsArr[1] ?
        _.map(buildVariantsArr[1].split(','), decodeURIComponent) : [];
      }
    }
  };

  ret.init = function(buildVariants, taskName, project) {
    // All build variants
    ret.buildVariants = buildVariants;
    ret.taskName = taskName;
    ret.project = project;
    ret.constraints = {
      low: Number.POSITIVE_INFINITY,
      high: 0
    };

    // Build Variant autocomplete state
    ret.buildVariantSearchString = "";
    ret.buildVariantSearchResults = [];
    ret.buildVariantSearchDisplay = false;

    ret.testsLoading = false;
    ret.taskMatchesFilter = {};

    if ($window.location.hash) {
      filterSerializer.deserialize(ret.locationHash.get());
      ret.filter.buildVariants = ret.filter.buildVariants || [];
    } else {
      ret.filter = {
        buildVariants: []
      };
    }
  };

  // Search through provided build variants' names. Used for the build variants
  // autocomplete
  ret.searchBuildVariants = function() {
    ret.buildVariantSearchResults = [];

    if (!ret.buildVariantSearchString) {
      return;
    }

    for (var i = 0; i < ret.buildVariants.length; ++i) {
      if (ret.buildVariants[i].toLowerCase().indexOf(ret.buildVariantSearchString.toLowerCase()) != -1) {
        ret.buildVariantSearchResults.push(ret.buildVariants[i]);
      }
    }

    ret.buildVariantSearchDisplay = true;
  };

  // Add a build variant to the filter
  ret.filterBuildVariant = function(buildVariant) {
    ret.filter.buildVariants.push(buildVariant);
    ret.buildVariantSearchString = "";
    ret.hideBuildVariantResults();
    ret.setLocationHash();

    // May need to query server again
    ret.queryServer();
  };

  // Remove the build variant at `index` from the filter
  ret.removeBuildVariant = function(index) {
    ret.filter.buildVariants.splice(index, 1);
    ret.queryServer();
    ret.buildVariantSearchString = "";
    ret.setLocationHash();
  };

  // Show the autocomplete build variant results
  ret.showBuildVariantResults = function() {
    ret.buildVariantSearchDisplay = true;
  };

  // Hide the build variant autocomplete results
  ret.hideBuildVariantResults = function() {
    ret.buildVariantSearchDisplay = false;
  };

  // Refresh the location
  ret.setLocationHash = function() {
    ret.locationHash.set(filterSerializer.serialize());
  };

  // Given the filter and low/high constraints, ask the server to find which
  // tasks match the given filter
  ret.queryServer = function() {
    ret.testsLoading = true;

    var filterStr = JSON.stringify(ret.filter);
    var uriFilterStr = encodeURIComponent(filterStr);
    $http.get(
      "/task_history/" +
      encodeURIComponent(ret.project) +
      '/' +
      encodeURIComponent(ret.taskName) +
      "/pickaxe" +
      "?low=" + ret.constraints.low +
      "&high=" + ret.constraints.high +
      "&filter=" + uriFilterStr
    ).then(function(resp) {
        var tasks = resp.data;
        ret.testsLoading = false;
        ret.taskMatchesFilter = {};
        if (tasks.length) {
          tasks.forEach(function(task) {
            ret.taskMatchesFilter[task.id] = true;
          });
        }
      }, function(resp) {
        ret.testsLoading = false;
        console.log("Error occurred when filtering tasks: `" + resp.headers + "`");
      });
    };

  return ret;
});

mciModule.controller('TaskHistoryController', function($scope, $window, $http,
  $filter, $timeout, taskHistoryFilter, mciTaskHistoryRestService, notificationService) {
  $scope.taskName = $window.taskName;
  $scope.variants = $window.variants;
  $scope.versions = [];
  $scope.failedTestsByTaskId = [];
  $scope.versionsByGitspec = {};
  $scope.tasksByVariantByCommit = [];
  $scope.taskHistoryFilter = taskHistoryFilter;
  $scope.isTaskGroupInactive = {};
  $scope.inactiveTaskGroupCount = {};
  $scope.exhaustedBefore = $window.exhaustedBefore;
  $scope.exhaustedAfter = $window.exhaustedAfter;
  $scope.selectedRevision = $window.selectedRevision;

  $scope.init = function(project) {
    $scope.project = project;
    $scope.taskHistoryFilter.init($scope.variants, $scope.taskName, project);

    /* Populate initial page data */
    buildVersionsByRevisionMap($window.versions, true);
    $scope.failedTestsByTaskId = $window.failedTasks;
    buildTasksByVariantCommitMap($window.tasksByCommit, true);

    var numVersions = $scope.versions.length;
    if (numVersions > 0) {
      $scope.firstVersion = $scope.versions[0].revision;
      $scope.lastVersion = $scope.versions[numVersions - 1].revision;
    }
  };

  function buildVersionsByRevisionMap(versions, before) {
    for (var i = 0; i < versions.length; ++i) {
      $scope.versionsByGitspec[versions[i].revision] = versions[i];

      if (versions[i].order > $scope.taskHistoryFilter.constraints.high) {
        $scope.taskHistoryFilter.constraints.high = versions[i].order;
      }
      if (versions[i].order < $scope.taskHistoryFilter.constraints.low) {
        $scope.taskHistoryFilter.constraints.low = versions[i].order;
      }
    }

    if (before) {
      Array.prototype.push.apply($scope.versions, versions);
    } else {
      Array.prototype.unshift.apply($scope.versions, versions);
    }

    // Make sure our filter gets updated against the server, because high
    // and low may have changed
    $scope.taskHistoryFilter.queryServer();
  }

  function buildTasksByVariantCommitMap(tasksByCommit, before) {
    if (!tasksByCommit || !tasksByCommit.length) {
      return;
    }

    $scope.isTaskGroupInactive = {};
    $scope.inactiveTaskGroupCount = {};

    var tasksByVariant = [];
    for (var i = 0; i < tasksByCommit.length; ++i) {
      var commitTasks = tasksByCommit[i];
      var buildVariantTaskMap = {};
      for (var j = 0; j < commitTasks.tasks.length; ++j) {
        buildVariantTaskMap[commitTasks.tasks[j].build_variant] =
        commitTasks.tasks[j];
      }

      tasksByVariant.push({
        _id: commitTasks._id,
        tasksByVariant: buildVariantTaskMap
      });
    }

    if (before) {
      Array.prototype.push.apply($scope.tasksByVariantByCommit, tasksByVariant);
    } else {
      Array.prototype.unshift.apply($scope.tasksByVariantByCommit, tasksByVariant);
    }


    var inactiveVersionSequenceStart = -1;
    _.each($scope.tasksByVariantByCommit, function(obj, index) {
      $scope.isTaskGroupInactive[obj._id] = isTaskGroupInactive($scope.variants, obj);
      if ($scope.isTaskGroupInactive[obj._id]) {
        if (inactiveVersionSequenceStart == -1) {
          inactiveVersionSequenceStart = index;
          $scope.inactiveTaskGroupCount[inactiveVersionSequenceStart] = 0;
        }
        ++$scope.inactiveTaskGroupCount[inactiveVersionSequenceStart];
      } else {
        inactiveVersionSequenceStart = -1;
      }
    });

  }

  $scope.taskMatchesFilter = function(task) {
    filter = $scope.taskHistoryFilter.filter;
    if (filter.buildVariants.length > 0) {
      if (filter.buildVariants.indexOf(task.build_variant) == -1) {
        return false;
      }
    }

    return true;
  };


  $scope.variantInFilter = function(variant) {
    filter = $scope.taskHistoryFilter.filter;
    if (filter.buildVariants.length > 0) {
      return filter.buildVariants.indexOf(variant) != -1;
    }

    return true;
  };

  $scope.taskGroupHasTaskMatchingFilter = function(variants, taskGroup) {
    for (var i = 0; i < variants.length; ++i) {
      var variant = variants[i];
      if (taskGroup.tasksByVariant[variant] &&
        $scope.taskMatchesFilter(taskGroup.tasksByVariant[variant])) {
        return true;
    }
  }

  return false;
};

var isTaskGroupInactive = function(variants, taskGroup) {
  for (var i = 0; i < variants.length; ++i) {
    var task = taskGroup.tasksByVariant[variants[i]];
    if (task && ['success', 'failed'].indexOf(task.status) != -1) {
      return false;
    }
  }

  return true;
};

$scope.getTestForVariant = function(testGroup, buildvariant) {
  return testGroup.tasksByVariant[buildvariant];
}

$scope.getVersionForCommit = function(gitspec) {
  return $scope.versionsByGitspec[gitspec];
}

$scope.getTaskTooltip = function(testGroup, buildvariant) {
  var task = testGroup.tasksByVariant[buildvariant];
  var tooltip = '';
  if (!task) {
    return
  }

  switch (task.status) {
    case 'failed':
    if ('task_end_details' in task && 'timed_out' in task.task_end_details && task.task_end_details.timed_out) {
      tooltip += 'Timed out (' + task.task_end_details.desc + ') in ' +
      $filter('stringifyNanoseconds')(task.time_taken);
    } else if (task._id in $scope.failedTestsByTaskId &&
      $scope.failedTestsByTaskId[task._id].length > 0) {
      var failedTests = $scope.failedTestsByTaskId[task._id];
      var failedTestLimit = 3;
      var displayedTests = [];
      for (var i = 0; i < failedTests.length; i++) {
        if (i < failedTestLimit) {
          displayedTests.push($filter('endOfPath')(failedTests[i]));
        }
      }
      tooltip += failedTests.length + ' ' + $filter('pluralize')(failedTests.length, 'test') +
      ' failed (' + $filter('stringifyNanoseconds')(task.time_taken) + ')\n';
      _.each(displayedTests, function(displayedTest) {
        tooltip += '- ' + displayedTest + '\n';
      });
    } else {
      tooltip = $filter('capitalize')(task.status) + ' (' +
        $filter('stringifyNanoseconds')(task.time_taken) + ')';
}
break;
case 'success':
tooltip = $filter('capitalize')(task.status) + ' (' +
  $filter('stringifyNanoseconds')(task.time_taken) + ')';
break;
default:
}
return tooltip;
}

$scope.getGridClass = function(cell) {
  if (cell) {
    if (cell.status == 'undispatched') {
      return 'undispatched ' + (cell.activated ? 'active' : 'inactive')
    }
    cell.task_end_details = cell.details;
    return $filter('statusFilter')(cell);
  }
  return 'skipped';
};


$scope.loadMore = function(before) {
  $scope.taskHistoryFilter.testsLoading = true;
  var revision = $scope.firstVersion;
  if (before) {
    revision = $scope.lastVersion;
  }

  mciTaskHistoryRestService.getTaskHistory(
    $scope.project,
    $scope.taskName, {
      format: 'json',
      revision: revision,
      before: before,
    }, {
      success: function(resp) {
        var data = resp.data;
        if (data.Versions) {
          buildVersionsByRevisionMap(data.Versions, before);

          var numVersions = data.Versions.length;
          if (numVersions > 0) {
            if (before) {
              $scope.lastVersion = data.Versions[numVersions - 1].revision;
              $scope.exhaustedBefore = data.ExhaustedBefore;
            } else {
              $scope.firstVersion = data.Versions[0].revision;
              $scope.exhaustedAfter = data.ExhaustedAfter;
            }
          }
        }

        if (data.Tasks) {
          buildTasksByVariantCommitMap(data.Tasks, before)
        }

          // add column highlighting to the new elements. use a $timeout to
          // let the new elements get created before the handlers are registered
          $timeout(function() {
            $window.addColumnHighlighting(true);
          }, 0);
        },

        error: function(resp) {
          notificationService.pushNotification('Error getting task history: ' + resp.data.error,'errorNearButton');
        }
      }
      );
};

$scope.hideInactiveVersions = {
  v: true,
  get: function() {
    return $scope.hideInactiveVersions.v;
  },
  toggle: function() {
    $scope.hideInactiveVersions.v = !$scope.hideInactiveVersions.v;
  }
};

$scope.hideUnmatchingVersions = {
  v: false,
  get: function() {
    return $scope.hideUnmatchingVersions.v;
  },
  toggle: function() {
    $scope.hideUnmatchingVersions.v = !$scope.hideUnmatchingVersions.v;
  },
  hidden: function(){
    return _.size($scope.taskHistoryFilter.filter.tests) == 0
  }
};
});



// function to add mouseover handlers for highlighting columns
function addColumnHighlighting(unbindPrevious) {
  $("div[class*='column-']").each(function(i, el) {
    var elClasses = $(el).attr("class").split(' ');
    var columnClass = null;
    _.each(elClasses, function(c) {
      if (c.indexOf('column-') === 0) {
        columnClass = c;
      }
    });

    if (!columnClass) {
      return;
    }

    // this is a little aggressive, but since we don't attach any
    // other handlers for these events anywhere it should be okay
    if (unbindPrevious) {
      $(el).off('mouseenter');
      $(el).off('mouseleave');
    }

    $(el).on("mouseenter", function() {
      $('.' + columnClass).addClass('highlight-column');
    });

    $(el).on("mouseleave", function() {
      $('.' + columnClass).removeClass('highlight-column');
    });
  });
};

// add column highlighting on document ready
$(document).ready(function() {
  addColumnHighlighting(false);
});
