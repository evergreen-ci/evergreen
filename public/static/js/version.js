mciModule.controller('VersionController', function($scope, $rootScope, $location, $http, $filter, $now, $window, notificationService, $mdDialog, mciSubscriptionsService, $mdToast) {
  var nsPerMs = 1000000
  $scope.canEdit = $window.canEdit
  $scope.jiraHost = $window.jiraHost;

  var dateSorter = function(a, b){ return (+a) - (+b) }
  $scope.tab = 0
  $scope.version = {};
  $scope.taskStatuses = {};
  $scope.triggers = [
    {
      trigger: "outcome",
      resource_type: "VERSION",
      label: "this version finishes",
    },
    {
      trigger: "failure",
      resource_type: "VERSION",
      label: "this version fails",
    },
    {
      trigger: "success",
      resource_type: "VERSION",
      label: "this version succeeds",
    },
    {
      trigger: "exceeds-duration",
      resource_type: "VERSION",
      label: "the runtime for this version exceeds some duration",
      extraFields: [
        {text: "Version duration (seconds)", key: "version-duration-secs", validator: validateDuration}
      ],
    },
    {
      trigger: "runtime-change",
      resource_type: "VERSION",
      label: "the runtime for this version changes by some percentage",
      extraFields: [
        {text: "Percent change", key: "version-percent-change", validator: validatePercentage}
      ],
    },
    {
      trigger: "outcome",
      resource_type: "BUILD",
      label: "a build-variant in this version finishes",
      regex_selectors: buildRegexSelectors(),
    },
    {
      trigger: "failure",
      resource_type: "BUILD",
      label: "a build-variant in this version fails",
      regex_selectors: buildRegexSelectors(),
    },
    {
      trigger: "success",
      resource_type: "BUILD",
      label: "a build-variant in this version succeeds",
      regex_selectors: buildRegexSelectors(),
    },
  ];
  hash = $location.hash();
  path = $location.path();
  $scope.collapsed = localStorage.getItem("collapsed") == "true";
  if (window.hasBanner && !isDismissed(bannerText())) {
    $("#drawer").addClass("bannerMargin");
    $("#content").addClass("bannerMargin");
  }

  // If a tab number is specified in the URL, parse it out and set the tab
  // number in the scope so that the correct tab is open when the page loads.
  if (path && !isNaN(parseInt(path.substring(1)))) {
    $scope.tab = parseInt(path.substring(1));
  } else if (!isNaN(parseInt(hash))) {
    $scope.tab = parseInt(hash);
  }

  $scope.$watch("collapsed", function() {
    localStorage.setItem("collapsed", $scope.collapsed);
  });

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

  $scope.addSubscription = function() {
    omitMethods = {};
    omitMethods[SUBSCRIPTION_JIRA_ISSUE] = true;
    omitMethods[SUBSCRIPTION_EVERGREEN_WEBHOOK] = true;
    promise = addSubscriber($mdDialog, $scope.triggers, omitMethods);

    $mdDialog.show(promise).then(function(data){
      if (data.resource_type === "VERSION") {
        addSelectorsAndOwnerType(data, "version", $scope.version.Version.id);

      }else {
        addInSelectorsAndOwnerType(data, "version", "version", $scope.version.Version.id);
      }
      $scope.saveSubscription(data);
    });
  };

  $scope.saveSubscription = function(subscription) {
    var success = function() {
      $mdToast.show({
        templateUrl: "/static/partials/subscription_confirmation_toast.html",
        position: "bottom right"
      });
    };
    var failure = function(resp) {
      notifier.pushNotification('Error saving subscriptions: ' + resp.data.error, 'notifyHeader');
    };
    mciSubscriptionsService.post([subscription], { success: success, error: failure });
  }

  $rootScope.$on("version_updated", function(e, newVersion){
    // cheat and copy over the patch info, since it never changes.
    newVersion.PatchInfo = $scope.version['PatchInfo']
    $scope.setVersion(newVersion);
  })

  $scope.setVersion = function(version) {
    $scope.version = version;

    $scope.commit = {
      message: $scope.version.Version.message,
      author: $scope.version.Version.author,
      author_email: $scope.version.Version.author_email,
      create_time: $scope.version.Version.create_time,
      gitspec: $scope.version.upstream?$scope.version.upstream.revision:$scope.version.Version.revision,
      repo_owner: $scope.version.upstream?$scope.version.upstream.owner:$scope.version.repo_owner,
      repo_name: $scope.version.upstream?$scope.version.upstream.repo:$scope.version.repo_name
    };

    $scope.taskStatuses = {};
    var taskNames = {};
    $scope.taskGrid = {};

    if (version.PatchInfo) {
      // setup diff data to use statusFilter
      for (var i = 0; i < version.PatchInfo.StatusDiffs.length; ++i) {
        var original = version.PatchInfo.StatusDiffs[i].diff.original;

        // in case the base task has not yet run
        if (_.size(original) !== 0) {
          version.PatchInfo.StatusDiffs[i].diff.original = {
            'task_end_details': original,
            'status': original.status,
          };
        }

        var patch = version.PatchInfo.StatusDiffs[i].diff.patch;

        // in case the patch task has not yet run
        if (_.size(patch) !== 0) {
          version.PatchInfo.StatusDiffs[i].diff.patch = {
            'task_end_details': patch,
            'status': patch.status,
          };
        }
      }
    }

    for (var i = 0; i < version.Builds.length; ++i) {
      row = {}
      $scope.taskStatuses[version.Builds[i].Build._id] = [];
      for (var j = 0; j < version.Builds[i].Tasks.length; ++j) {
        row[version.Builds[i].Tasks[j].Task.display_name] = version.Builds[i].Tasks[j].Task;
        $scope.taskStatuses[version.Builds[i].Build._id].push({
          "class": $filter('statusFilter')(version.Builds[i].Tasks[j].Task),
          "tooltip": version.Builds[i].Tasks[j].Task.display_name + " - " + $filter('statusLabel')(version.Builds[i].Tasks[j].Task),
          "link": "/task/" + version.Builds[i].Tasks[j].Task.id
        });
        taskNames[version.Builds[i].Tasks[j].Task.display_name] = 1;
      }
      $scope.taskGrid[version.Builds[i].Build.display_name] = row;
    }
    $scope.taskNames = Object.keys(taskNames).sort()
    $scope.lastUpdate = $now.now();

    //calculate makespan and total processing time for the version
    var nonZeroTimeFilter = function(y){return (+y) != (+new Date(0))};

    var tasks = _.filter(version.Builds.map(function(x){ return _.pluck(x['Tasks'] || [], "Task") }).reduce(function(x,y){return x.concat(y)}, []), function(task){
      return task.status == "success" || task.status == "failed"
    });
    var taskStartTimes = _.filter(_.pluck(tasks, "start_time").map(function(x){return new Date(x)}), nonZeroTimeFilter).sort(dateSorter);
    var taskEndTimes = _.filter(_.pluck(tasks, "finish_time").map(function(x){return new Date(x)}), nonZeroTimeFilter).sort(dateSorter);

    if(taskStartTimes.length == 0 || taskEndTimes.length == 0) {
        $scope.makeSpanMS = 0;
    }else {
        $scope.makeSpanMS = taskEndTimes[taskEndTimes.length-1] - taskStartTimes[0];
    }

    var availableTasks = _.filter(tasks, function(t){
      return +new Date(t.start_time) != +new Date(0);
    })

    $scope.totalTimeMS = _.reduce(_.pluck(availableTasks, "time_taken"), function(x, y){return x+y}, 0) / nsPerMs;
  };

  $scope.getGridLink = function(bv, test) {
    if (!(bv in $scope.taskGrid)) {
      return '#';
    }
    var cell = $scope.taskGrid[bv][test]
    if (!cell) {
      return '#';
    }
    return '/task/' + cell.id;
  }

  $scope.getGridClass = function(bv, test) {
    var returnval = '';
    var bvRow = $scope.taskGrid[bv];
    if (!bvRow) return 'skipped';
    var cell = bvRow[test];
    if (!cell) return 'skipped';
    if (cell.status == 'started' || cell.status == 'dispatched') {
      return 'started';
    } else if (cell.status == 'undispatched') {
      return 'undispatched ' + (cell.activated ? 'active' : ' inactive');
    } else if (cell.status == 'failed') {
      if ('task_end_details' in cell) {
        if ('type' in cell.task_end_details) {
          if (cell.task_end_details.type == 'system') {
            return 'system-failed';
          }
          if (cell.task_end_details.type == 'setup') {
            return 'setup-failed';
          }
        }
      }
      return 'failure';
    } else if (cell.status == 'success') {
      return 'success';
    }
  }

  $scope.load = function() {
    $http.get('/version_json/' + $scope.version.Version.id).then(
    function(resp) {
      var data = resp.data;
      if (data.error) {
        notificationService.pushNotification(data.error);
      } else {
        $scope.setVersion(data);
      }
    },
    function(resp) {
      notificationService.pushNotification("Error occurred - " + resp.data.error);
    });
  };

  $scope.setVersion($window.version);
  $scope.plugins = $window.plugins;
});


mciModule.controller('VersionHistoryDrawerCtrl', function($scope, $window, $filter, $timeout, historyDrawerService) {
  const APPROX_REVISION_ITEM_HEIGHT = 17

  // cache the task being displayed on the page
  $scope.version = $window.version;

  // cache the element for the content of the drawer
  var drawerContentsEl = $('#drawer-contents');

  // is the specified revision the one with the current task in it?
  $scope.isCurrent = function(revision) {
    return revision.revision === $scope.version.Version.revision;
  }

  // helper to convert the history fetched from the backend into revisions,
  // grouped by date, for front-end display
  function groupHistory(history) {
    // group the revisions by date, ordered backwards by date
    var groupedRevisions = [];
    var datesSeen = {}; // to avoid double-entering dates
    history.forEach(function(revision) {
      var date = revision.create_time.substring(0, 10);

      // if we haven't seen the date, add a new entry for it
      if (!datesSeen[date]) {
        groupedRevisions.push({
          date: date,
          revisions: [],
        });
        datesSeen[date] = true;
      }

      // push the revision onto its appropriate date group (always the
      // last in the list)
      groupedRevisions[groupedRevisions.length - 1].revisions.push(revision);
    });
    return groupedRevisions;
  }

  // make a backend call to get the drawer contents
  function fetchHistory() {
    historyDrawerService.fetchVersionHistory($scope.version.Version.id, 'surround', 20, {
      success: function(resp) {
        var data = resp.data;

        // save the revisions as a list
        $scope.revisions = data.revisions;

        // group the history by revision, and save it
        $scope.groupedRevisions = groupHistory(data.revisions);

        // scroll to the relevant element
        $timeout(
          function() {
            var currentRevisionDomEl = $('.drawer-item-highlighted')[0];
            if (!currentRevisionDomEl) {
              return;
            }
            var offsetTop = $(currentRevisionDomEl).position().top;
            var drawerContentsHeight = drawerContentsEl.height();
            if (offsetTop >= drawerContentsHeight) {
              drawerContentsEl.scrollTop(offsetTop);
            }
          }, 500)
      },
      error: function(data) {
        console.log('error fetching history: ' + JSON.stringify(data));
      }
    });

  }

  // function fired when scrolling up hits the top of the frame,
  // loads more revisions asynchronously
  var fetchLaterRevisions = _.debounce(
    function() {
      // get the most recent revision in the history
      var mostRecentRevision = ($scope.revisions && $scope.revisions[0]);

      // no history
      if (!mostRecentRevision) {
        return;
      }

      // get a version id from it
      var anchorId = mostRecentRevision.version_id;

      historyDrawerService.fetchVersionHistory(anchorId, 'after', 20, {
        success: function(resp) {
          var data = resp.data;
          // no computation necessary
          if (!data) {
            return;
          }

          // place on the beginning of the stored revisions
          $scope.revisions = data.revisions.concat($scope.revisions);

          // regroup
          $scope.groupedRevisions = groupHistory($scope.revisions);

          // Scroll down by rough offset calculation
          drawerContentsEl.scrollTop(APPROX_REVISION_ITEM_HEIGHT * data.revisions.length);

        },
        error: function(data) {
          console.log('error fetching later revisions: ' + JSON.stringify(data));
        }
      })
    }, 500, true);

  // function fired when scrolling down hits the bottom of the frame,
  // loads more revisions asynchronously
  var fetchEarlierRevisions = _.debounce(

    function() {
      // get the least recent revision in the history
      var leastRecentRevision = ($scope.revisions &&
        $scope.revisions[$scope.revisions.length - 1]);

      // no history
      if (!leastRecentRevision) {
        return;
      }

      // get a version id from it
      var anchorId = leastRecentRevision.version_id;

      historyDrawerService.fetchVersionHistory(
        anchorId,
        'before',
        20, {
          success: function(resp) {
            var data = resp.data;
            // no computation necessary
            if (!data) {
              return;
            }

            // place on the end of the stored revisions
            $scope.revisions = $scope.revisions.concat(data.revisions);

            // regroup
            $scope.groupedRevisions = groupHistory($scope.revisions);

          },
          error: function(data) {
            console.log('error fetching earlier revisions: ' + JSON.stringify(data));
          }
        }
      )

    },
    500,
    true
  );

  // make the initial call to fetch the history
  fetchHistory();

  /* infinite scroll stuff */

  // the drawer header element
  var drawerHeaderEl = $('#drawer-header');

  // the filled part of the drawer
  var drawerFilledEl = $('#drawer-filled');

  // scrolling function to fire if the element is not actually scrollable
  // (does not overflow its div)
  var loadMoreSmall = _.debounce(
    function(e) {
      var evt = window.event || e;
      if (evt.wheelDelta) {
        if (evt.wheelDelta < 0) {
          fetchEarlierRevisions();
        } else {
          fetchLaterRevisions();
        }
      }

      // firefox: mouse wheel info is in a subobject called detail
      else if (evt.detail && evt.detail.wheelDelta) {
        if (evt.detail.wheelDelta < 0) {
          fetchLaterRevisions();
        } else {
          fetchEarlierRevisions();
        }
      }
    },
    500,
    true
  );

  // activates infinite scrolling if the drawer contents are not large enough
  // to be normally scrollable
  var smallScrollFunc = function(e) {
    if (drawerFilledEl.height() < drawerContentsEl.height()) {
      loadMoreSmall(e);
    }
  }

  drawerContentsEl.on('mousewheel DOMMouseScroll onmousewheel', _.debounce(smallScrollFunc, 100));

  // scrolling function to fire if the element is scrollable (it overflows
  // its div)
  var bigScrollFunc = function() {
    if (drawerContentsEl.scrollTop() === 0) {
      // we hit the top of the drawer
      fetchLaterRevisions();
    } else if (
      drawerContentsEl.scrollTop() + 10 >= drawerContentsEl[0].scrollHeight - drawerContentsEl.height()
    ) {
      // we hit the bottom of the drawer
      fetchEarlierRevisions();
    }
  }

  // set up infinite scrolling on the drawer element
  drawerContentsEl.scroll(bigScrollFunc);

  var eopFilter = $filter('endOfPath');
  $scope.failuresTooltip = function(failures) {
    return _.map(failures, function(failure) {
      return eopFilter(failure);
    }).join('\n');
  }

});
