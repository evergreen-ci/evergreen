mciModule.controller('TaskHistoryDrawerCtrl', function($scope, $window, $location, $filter, $timeout, historyDrawerService) {

  // cache the task being displayed on the page
  $scope.task = $window.task_data;

  // cache the element for the content of the drawer
  var drawerContentsEl = $('#drawer-contents');

  // is the specified revision the one with the current task in it?
  $scope.isCurrent = function(revision) {
    return revision.revision === $scope.task.gitspec;
  }

  // helper to convert the history fetched from the backend into revisions,
  // grouped by date, for front-end display
  function groupHistory(history) {

      // group the revisions by date, ordered backwards by date
      var groupedRevisions = [];
      var datesSeen = {}; // to avoid double-entering dates
      history.forEach(function(revision) {
        var date = revision.push_time.substring(0, 10);

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
    historyDrawerService.fetchTaskHistory($scope.task.id, 'surround', 20,
      {
        success: function(data) {

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
          console.log('error fetching history: ' + data);
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
        return
      }

      // get a task id from it 
      var anchorId = mostRecentRevision.task.id;

      historyDrawerService.fetchTaskHistory(anchorId, 'after', 20,
        {
          success: function(data) {
           // no computation necessary
           if (!data) {
             return
           }

           // place on the beginning of the stored revisions
           $scope.revisions = data.revisions.concat($scope.revisions);

           // regroup
           $scope.groupedRevisions = groupHistory($scope.revisions);

          },
          error: function(data) {
            console.log('error fetching later revisions: ' + data);
          }
        })
    }, 500, true);

  // function fired when scrolling down hits the bottom of the frame,
  // loads more revisions asynchronously
  var fetchEarlierRevisions = _.debounce(
      
    function() {
      // get the least recent revision in the history  
      var leastRecentRevision = ($scope.revisions && 
          $scope.revisions[$scope.revisions.length-1]);

      // no history
      if (!leastRecentRevision) {
        return
      }

      // get a task id from it
      var anchorId = leastRecentRevision.task.id;

      mciTaskDrawerRestService.fetchHistory(
        anchorId,
        'before',
        20, 
        {
          success: function(data) {
            // no computation necessary
            if (!data) {
              return
            }

            // place on the end of the stored revisions  
            $scope.revisions = $scope.revisions.concat(data.revisions);

            // regroup 
            $scope.groupedRevisions = groupHistory($scope.revisions);

          },
          error: function(data) {
            console.log('error fetching earlier revisions: ' + data);
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

      // firefox
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

  drawerContentsEl.on('mousewheel DOMMouseScroll onmousewheel', smallScrollFunc);

  // scrolling function to fire if the element is scrollable (it overflows 
  // its div)
  var bigScrollFunc = function() {
    if (drawerContentsEl.scrollTop() === 0) {
      // we hit the top of the drawer
      fetchLaterRevisions();

    } else if (drawerContentsEl.scrollTop() + 10 >= 
        drawerContentsEl[0].scrollHeight - drawerContentsEl.height()) {

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

mciModule.controller('TaskCtrl', function($scope, $now, $timeout, $interval, md5, $filter, $window, $http, $locationHash) {
  $scope.userTz = $window.userTz;

  var hash = $locationHash.get();
  $scope.hash = hash;

  $scope.setSortBy = function(order) {
    $scope.sortBy = order;
    hash.sort = order.name;
    $locationHash.set(hash);
  };

  $scope.linkToTest = function(testName) {
    if (hash.test === testName) {
      delete hash.test;
      $locationHash.set(hash);
    } else {
      hash.test = testName;
      $locationHash.set(hash);
    }
  };

  $scope.setTask = function(task) {
    $scope.task = task;
    $scope.md5 = md5;
    $scope.maxTests = 1;

    $scope.sortOrders = [
      { name : "Status",      by : "status",        reverse : false },
      { name : "Name",        by : "display_name",  reverse : false },
      { name : "Time Taken",  by : "time_taken",    reverse : true  },
      { name : "Sequence",    by : "",              reverse : true  }
    ];
    (task.test_results || []).forEach(function(testResult) {
      testResult.time_taken = testResult.end - testResult.start;
      testResult.display_name = $filter('endOfPath')(testResult.test_file);
    });

    if (hash.sort) {
      var index = _.indexOf(_.pluck($scope.sortOrders, 'name'), hash.sort);
      if (index != -1) {
        $scope.sortBy = $scope.sortOrders[index];
      }
    }
    if (task.execution > 0) {
        $scope.pastExecutions = [ ]
        for (var i = 0; i < task.execution; i++) {
            $scope.pastExecutions.push(i);
        }
    }

    $scope.sortBy = $scope.sortOrders[0];
    $scope.dependencies = [];
    $http.get('/task/dependencies/' + task.id + '/' + task.execution ).
        success(function(data) {
          $scope.dependencies = data;
        }).
        error(function(data) {
          alert("Error getting task dependencies: " + JSON.stringify(data));
        });

    $scope.timeTaken = $scope.task.time_taken

    if ($scope.task.status === 'started' || $scope.task.status === 'dispatched') {
      updateFunc = function() {
        $scope.task.current_time += 1000000000; // 1 second
        $scope.timeTaken = $scope.task.current_time - $scope.task.start_time;
        $scope.timeToCompletion = $scope.task.expected_duration - ($scope.task.current_time - $scope.task.start_time);
        if ($scope.timeToCompletion < 0) {
          $scope.timeToCompletion = "unknown";
        }
      }
      updateFunc();
      var updateTimers = $interval(updateFunc, 1000);
    }
  };

  $scope.setTask($window.task_data);
  $scope.plugins = $window.plugins

  $scope.maxTestTime = 1;

  $scope.lastUpdate = $now.now();

  $scope.githubLink = function() {
    if (!$scope.task) {
      return '#';
    }
    var projectComponents = $scope.task.project.split('-');
    if (projectComponents[projectComponents.length-1] === 'master') {
      projectComponents = projectComponents.slice(0, projectComponents.length-1);
    }
    return '//github.com/' + projectComponents.join('/') + '/commit/' + $scope.task.gitspec; 
  };

  // Returns URL to task history page with a filter on the particular test
  // and test status enabled.
  $scope.getTestHistoryUrl = function(project, task, test) {
    return '/task_history/' +
      encodeURIComponent(project) + '/' +
      encodeURIComponent(task.display_name) + '?revision=' +
      encodeURIComponent(task.gitspec) + '#' +
      encodeURIComponent(test.display_name) + '=' +
      encodeURIComponent(test.status);
  };

});

mciModule.directive('testsResultsBar', function($filter) {
  return {
    scope: true,
    link: function(scope, element, attrs) {
      scope.$watch(attrs.testsResultsBar, function(testResults) {
        if (testResults) {
          var numSuccess = 0;
          var numFailed = 0;
          var successTimeTaken = 0;
          var failureTimeTaken = 0;
          _.each(testResults, function(result) {
            switch(result.status) {
              case "pass":
                numSuccess++;
                successTimeTaken += (result.end - result.start);
                break;
              case "fail":
                numFailed++;
                failureTimeTaken += (result.end - result.start);
                break;
            }
          });

          successTimeTaken = $filter('nanoToSeconds')(successTimeTaken)
          failureTimeTaken = $filter('nanoToSeconds')(failureTimeTaken)
          var successTitle = numSuccess + " test" + (numSuccess == 1 ? "" : "s") + " succeeded in " + $filter('stringifyNanoseconds')(successTimeTaken);
          var failedTitle = numFailed + " test" + (numFailed == 1 ? "" : "s") + " failed in " + $filter('stringifyNanoseconds')(failureTimeTaken);
          element.html( '<div class="progress-bar progress-bar-success" role="progressbar" style="width: ' + (numSuccess / testResults.length * 100) + '%" data-animation="" data-toggle="tooltip" title="' + successTitle + '"></div>' +
                        '<div class="progress-bar progress-bar-danger" role="progressbar" style="width: ' + (numFailed / testResults.length * 100) + '%"  data-animation="" data-toggle="tooltip" title="' + failedTitle + '"></div>')

          $(element.children('*[data-toggle="tooltip"]')).each(function(i, el) {
            $(el).tooltip();
          });

          scope.barWidth = testResults.length / scope.maxTests * 90;
        }
      });
    }
  }
});

mciModule.directive('testResults', function() {
  return {
    scope: true,
    link: function(scope, element, attrs) {
      scope.maxTestTime = 1;
      scope.$watch(attrs.testResults, function(testResults) {
        if (testResults) {
          for (var i = 0; i < testResults.length; i++) {
            var timeTaken = testResults[i].end - testResults[i].start;
            if (scope.maxTestTime < timeTaken) {
              scope.maxTestTime = timeTaken;
            }
          }
        }
      });
    }
  }
});

mciModule.directive('testResultBar', function($filter) {
  return {
    scope: true,
    link: function(scope, element, attrs) {
      scope.$watch(attrs.testResultBar, function(testResult) {
        var timeTaken = testResult.end - testResult.start;
        scope.timeTaken = timeTaken;

        switch (testResult.status) {
          case "pass":
            scope.progressBarClass = "progress-bar-success";
            break;
          case "fail":
            scope.progressBarClass = "progress-bar-danger";
            break;
          default:
            scope.progressBarClass = "progress-bar-default"
        }

        var timeInNano = scope.timeTaken * 1000 * 1000 * 1000;
        $(element).tooltip({
          title: $filter('stringifyNanoseconds')(timeInNano),
          animation: false,
        });
      });

      scope.$watch('maxTestTime', function(maxTestTime) {
        scope.barWidth =  scope.timeTaken / maxTestTime * 90;
        if (scope.barWidth < 5) {
          scope.barWidth = 5;
        }
      });
    }
  }
});

mciModule.controller('TaskLogCtrl', function($scope, $timeout, $http, $location, $window, $filter) {
  $scope.logs = 'Loading...';
  $scope.task = {};
  $scope.eventLogs = 'EV';
  $scope.systemLogs = 'S';
  $scope.agentLogs = 'E';
  $scope.taskLogs = 'T';
  $scope.allLogs = 'ALL';
  $scope.userTz = $window.userTz;

  var logSpec = $location.path().split('/');
  $scope.currentLogs = logSpec[2] || $scope.taskLogs;

  $scope.$watch('currentLogs', function() {
    $scope.getLogs();
  });

  $scope.setCurrentLogs = function(currentLogs) {
    $scope.logs = 'Loading...';
    $scope.currentLogs = currentLogs;
    $location.path('log/' + currentLogs);
  };

  $scope.formatTimestamp = function(logEntry, minVersion) {
    if (!logEntry.version || logEntry.version < minVersion) {
      return '';
    }

    var converter = $filter('convertDateToUserTimezone');
    var format = 'YYYY/MM/DD HH:mm:ss.SSS';
    var timestamp = converter(logEntry.timestamp, $scope.userTz, format);
    return '[' + timestamp + '] '
  }

  $scope.getLogs = function() {
    $http.get('/json/task_log/' + $scope.taskId + '/' + $scope.task.execution + '?type=' + $scope.currentLogs).
      success(function(data, status) {
        if($scope.currentLogs == $scope.eventLogs){
          $scope.eventLogData = data.reverse()
        } else{
          if (data && data.LogMessages) {
            //read the log messages out, and reverse their order (since they are returned backwards)
            $scope.logs = _.map(data.LogMessages, function(entry) {
              var msg = entry.m.replace(/&#34;/g, '"');
              var date = new Date(entry.ts);
              return { message: msg, severity: entry.s, timestamp: date, version: entry.v };
            });
          } else {
            $scope.logs = [];
          }
        }
      }).
      error(function(jqXHR, status, errorThrown) {
        //alert('Error retrieving logs: ' + jqXHR);
      });

    // If we already have an outstanding timeout, cancel it
    if ($scope.getLogsTimeout) {
      $timeout.cancel($scope.getLogsTimeout);
    }

    $scope.getLogsTimeout = $timeout(function() {
      $scope.getLogs();
    }, 5000);
  };

  $scope.getRawLogLink = function() {
    if ($scope.currentLogs === $scope.eventLogs) {
      return '/event_log/task/' + $scope.taskId;
    } else {
      return '/task_log_raw/' + $scope.taskId + '/' + $scope.task.execution + '?type=' + $scope.currentLogs;
    }
  };

  $scope.setTask = function(task) {
    $scope.task = task;
    $scope.taskId = task.id;
  };

  $scope.setTask($window.task_data);

});
