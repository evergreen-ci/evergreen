mciModule.controller('TaskHistoryDrawerCtrl', function($scope, $window, $location, $filter, $timeout, taskHistoryDrawerService) {
  const APPROX_TASK_ITEM_HEIGHT = 17
  // cache the task being displayed on the page
  $scope.task = $window.task_data;

  // cache the element for the content of the drawer
  var drawerContentsEl = $('#drawer-contents');

  // is the specified revision the one with the current task in it?
  $scope.isCurrent = function(revision) {
    return revision.revision === $scope.task.gitspec;
  }

  if (window.hasBanner && !isDismissed(bannerText())) {
    $("#drawer").addClass("bannerMargin");
    $("#page-content").addClass("bannerMargin");
    $("#content").addClass("bannerMargin");
  }

  // handle resizing of left sidebar. Since everything on this page is in containers
  // that have position:absolute, we need to manually adjust widths/positions like this
  var isResizing = false;
  var lastXPos = 0;
  $(function () {
    var container = $('#page'),
        left = $('#drawer'),
        right = $('#page-content'),
        handle = $('#drag-bar');

    handle.on('mousedown', function (e) {
        isResizing = true;
        lastXPos = e.clientX;
    });

    $(document).on('mousemove', function (e) {
        if (!isResizing)
            return;

        var offset = e.clientX - container.offset().left;
        left.css('width', offset);
        right.css('left', offset);
    }).on('mouseup', function (e) {
        isResizing = false;
    });
  });

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
    taskHistoryDrawerService.fetchTaskHistory($scope.task.version_id, $scope.task.build_variant, $scope.task.display_name, 'surround', 20, {
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
        error: function(resp) {
          console.log('error fetching history: ' + JSON.stringify(resp.data));
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

        taskHistoryDrawerService.fetchTaskHistory(mostRecentRevision.version_id, $scope.task.build_variant, $scope.task.display_name, 'after', 20, {
          success: function(resp) {
            var data = resp.data;
            // no computation necessary
            if (!data) {
              return
            }

            // place on the beginning of the stored revisions
            $scope.revisions = data.revisions.concat($scope.revisions);

            // regroup
            $scope.groupedRevisions = groupHistory($scope.revisions);

            // Scroll down by rough offset calculation
            drawerContentsEl.scrollTop(APPROX_TASK_ITEM_HEIGHT * data.revisions.length);
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
              return
            }

            taskHistoryDrawerService.fetchTaskHistory(leastRecentRevision.version_id, $scope.task.build_variant, $scope.task.display_name, 'before', 20, {
                success: function(resp) {
                  var data = resp.data;
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

        drawerContentsEl.on('mousewheel DOMMouseScroll onmousewheel', _.debounce(smallScrollFunc, 100));

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

      mciModule.controller('TaskCtrl', function($scope, $rootScope, $now, $timeout, $interval, md5, $filter, $window,
      $http, $locationHash, $mdDialog, mciSubscriptionsService, notificationService, $mdToast, mciTasksRestService) {
        $scope.userTz = $window.userTz;
        $scope.haveUser = $window.have_user;
        $scope.taskHost = $window.taskHost;
        $scope.jiraHost = $window.jiraHost;
        $scope.isAdmin = $window.isAdmin;

        $scope.triggers = [
          {
            trigger: "outcome",
            resource_type: "TASK",
            label: "this task finishes",
          },
          {
            trigger: "failure",
            resource_type: "TASK",
            label: "this task fails",
          },
          {
            trigger: "success",
            resource_type: "TASK",
            label: "this task succeeds",
          },
          {
            trigger: "exceeds-duration",
            resource_type: "TASK",
            label: "the runtime for this task exceeds some duration",
            extraFields: [
              {text: "Task duration (seconds)", key: "task-duration-secs", validator: validateDuration}
            ]
          },
          {
            trigger: "runtime-change",
            resource_type: "TASK",
            label: "this task success and its runtime changes by some percentage",
            extraFields: [
              {text: "Percent change", key: "task-percent-change", validator: validatePercentage}
            ]
          },
        ];

        $scope.addSubscription = function() {
          omitMethods = {};
          omitMethods[SUBSCRIPTION_JIRA_ISSUE] = true;
          omitMethods[SUBSCRIPTION_EVERGREEN_WEBHOOK] = true;
          promise = addSubscriber($mdDialog, $scope.triggers, omitMethods);

          $mdDialog.show(promise).then(function(data){
            addSelectorsAndOwnerType(data, "task", $scope.task.id);
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
            notificationService.pushNotification('Error saving subscriptions: ' + resp.data.error, 'errorHeader');
          };
          mciSubscriptionsService.post([subscription], { success: success, error: failure });
        }

        $scope.overrideDependencies = function() {
          mciTasksRestService.takeActionOnTask(
            $scope.task.id,
            'override_dependencies',
            {},
            {
              success: function(resp) {
                $window.location.reload();
              },
              error: function(resp) {
                notificationService.pushNotification('Error overriding dependencies: ' + resp.data, 'errorModal');
              }
            }
          );
        }

        // Returns true if 'testResult' represents a test failure, and returns false otherwise.
        $scope.hasTestFailureStatus = function hasTestFailureStatus(testResult) {
          var failureStatuses = ['fail', 'silentfail'];
          return failureStatuses.indexOf(testResult.test_result.status) >= 0;
        };

        $scope.isSuccessful = function(testResult) {
          return testResult.test_result.status === 'pass';
        };

        $scope.getURL = function(testResult, isRaw) {
          var url = (isRaw) ? testResult.url_raw : testResult.url;

          if (url != '') {
            return url;
          }

          var logid = testResult.log_id;
          var linenum = testResult.line_num || 0;

          url = '/test_log/' + logid + '#L' + linenum;
          if (isRaw) {
            url = '/test_log/' + logid + '?raw=1';
          }

          return url;
        };

        $scope.execTaskUrl = function(taskId, execution) {
          if (execution >= 0) {
            return '/task/' + taskId + '/' + execution;
          }
          return '/task/' + taskId;
        };

        $scope.hideURL = function(testResult, isRaw) {
          var url = isRaw ? testResult.url_raw : testResult.url;
          return !((url != '') || (testResult.log_id));
        };

        $scope.hasBothURL = function(testResult) {
          return !($scope.hideURL(testResult) || $scope.hideURL(testResult,'raw'))
        };

        var hash = $locationHash.get();
        $scope.hash = hash;

        $scope.getSpawnLink = function(){
          if(!$scope.haveUser) { // user is not logged in, so we won't provide a link.
          return ""
        }
        if(!$scope.taskHost || $scope.taskHost.distro.provider == "static" || $scope.taskHost.distro.provider == "docker" || !$scope.taskHost.distro.spawn_allowed){
          return ""
        }
        return "/spawn?distro_id=" + $scope.taskHost.distro._id + "&task_id=" + $scope.task.id
      }

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

        /**
        * Defines the sort order for a test's status.
        */
        function ordinalForTestStatus(task) {
          var orderedTestStatuses = ['fail', 'silentfail', 'pass', 'skip'];
          return orderedTestStatuses.indexOf(task.test_result.status);
        }

        $scope.sortOrders = [{
          name: 'Status',
          by: [ordinalForTestStatus, 'test_result.display_name'],
          reverse: false
        }, {
          name: 'Name',
          by: ['test_result.display_name'],
          reverse: false
        }, {
          name: 'Time Taken',
          by: ['test_result.time_taken', 'test_result.display_name'],
          reverse: true
        }, {
          name: 'Sequence',
          by: ['test_result'],
          reverse: false
        }];

        var totalTestTime = 0;
        (task.test_results || []).forEach(function(t) {
          var testResult = t.test_result;
          testResult.time_taken = testResult.end - testResult.start;
          totalTestTime += testResult.time_taken;
          testResult.display_name = $filter('endOfPath')(testResult.test_file);
        });
        $scope.totalTestTimeNano = totalTestTime * 1000 * 1000 * 1000;

        if (hash.sort) {
          var index = _.indexOf(_.pluck($scope.sortOrders, 'name'), hash.sort);
          if (index != -1) {
            $scope.sortBy = $scope.sortOrders[index];
          }
        }

        if (task.execution > 0 || task.archived) {
          $scope.otherExecutions = _.range(task.total_executions + 1)
        }

        $scope.sortBy = $scope.sortOrders[0];

        $scope.isMet = function(dependency) {
          // check if a dependency is met, unmet, or in progress
          if (dependency.task_waiting == "blocked") {
            return "unmet";
          }
          if (dependency.status != "failed" && dependency.status != "success") {
            // if we didn't succeed or fail, don't report anything
            return "";
          }
          if (dependency.status == dependency.required || dependency.required == "*") {
            return "met";
          }
          return "unmet";
        };

        $scope.timeTaken = $scope.task.time_taken

        if ($scope.task.patch_info) {
          $scope.baseTimeTaken = $scope.task.patch_info.base_time_taken;
        }

        if ($scope.task.status != 'failed' && $scope.task.status != 'success') {
          updateFunc = function() {
            $scope.task.current_time += 1000000000; // 1 second
            $scope.timeTaken = $scope.task.current_time - $scope.task.start_time;
            $scope.timeToCompletion = $scope.task.expected_duration - ($scope.task.current_time - $scope.task.start_time);
            if ($scope.timeToCompletion < 0) {
              $scope.timeToCompletion = 'unknown';
            }
            if ($scope.task.status === 'undispatched'){
              $scope.timeToCompletion = $scope.task.expected_duration;
            }
          }
          updateFunc();
          var updateTimers = $interval(updateFunc, 1000);
        }
      };

      $rootScope.$on("task_updated", function(e, newTask){
        newTask.version_id = $scope.task.version_id;
        newTask.message = $scope.task.message;
        newTask.author = $scope.task.author;
        newTask.author_email = $scope.task.author_email;
        newTask.min_queue_pos = $scope.task.min_queue_pos;
        newTask.patch_info = $scope.task.patch_info;
        newTask.build_variant_display = $scope.task.build_variant_display;
        newTask.depends_on = $scope.task.depends_on;
        $scope.setTask(newTask);
      })

      $scope.setTask($window.task_data);
      $scope.plugins = $window.plugins

      $scope.maxTestTime = 1;

      $scope.lastUpdate = $now.now();

      $scope.githubLink = function() {
        if (!$scope.task) {
          return '#';
        }
        var projectComponents = $scope.task.project.split('-');
        if (projectComponents[projectComponents.length - 1] === 'master') {
          projectComponents = projectComponents.slice(0, projectComponents.length - 1);
        }
        return '//github.com/' + projectComponents.join('/') + '/commit/' + $scope.task.gitspec;
      };

      // Returns URL to task history page with a filter on the particular test
      // and test status enabled.
      $scope.getTestHistoryUrl = function(project, task, test, taskName) {
        if (!taskName || taskName === "") {
          taskName = task.display_name;
        }
        var url = '/task_history/' +
        encodeURIComponent(project) + '/' +
        encodeURIComponent(taskName) + '?revision=' +
        encodeURIComponent(task.gitspec);
        if (test) {
          url += '#' + encodeURIComponent(test.display_name) + '=' +
          encodeURIComponent(test.status);
        }
        return url
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
                switch (result.status) {
                  case 'pass':
                  numSuccess++;
                  successTimeTaken += (result.end - result.start);
                  break;
                  case 'fail':
                  case 'silentfail':
                  numFailed++;
                  failureTimeTaken += (result.end - result.start);
                  break;
                }
              });

              successTimeTaken = $filter('nanoToSeconds')(successTimeTaken)
              failureTimeTaken = $filter('nanoToSeconds')(failureTimeTaken)
              var successTitle = numSuccess + ' test' + (numSuccess == 1 ? '' : 's') + ' succeeded in ' + $filter('stringifyNanoseconds')(successTimeTaken);
              var failedTitle = numFailed + ' test' + (numFailed == 1 ? '' : 's') + ' failed in ' + $filter('stringifyNanoseconds')(failureTimeTaken);
              element.html('<div class="progress-bar progress-bar-success" role="progressbar" style="width: ' + (numSuccess / testResults.length * 100) + '%" data-animation="" data-toggle="tooltip" title="' + successTitle + '"></div>' +
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
                var timeTaken = testResults[i].test_result.end - testResults[i].test_result.start;
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
              case 'pass':
              scope.progressBarClass = 'progress-bar-success';
              break;
              case 'fail':
              scope.progressBarClass = 'progress-bar-danger';
              break;
              case 'silentfail':
              scope.progressBarClass = 'progress-bar-silently-failed';
              break;
              default:
              scope.progressBarClass = 'progress-bar-default';
            }

            var timeInNano = scope.timeTaken * 1000 * 1000 * 1000;
            $(element).tooltip({
              title: $filter('stringifyNanoseconds')(timeInNano),
              animation: false,
            });
          });

          scope.$watch('maxTestTime', function(maxTestTime) {
            scope.barWidth = scope.timeTaken / maxTestTime * 90;
            if (scope.barWidth < 5) {
              scope.barWidth = 5;
            }
          });
        }
      }
    });

    mciModule.controller('TaskLogCtrl', ['$scope', '$timeout', '$http', '$location', '$window', '$filter', 'notificationService', function($scope, $timeout, $http, $location, $window, $filter, notifier) {
      $scope.logs = 'Loading...';
      $scope.task = {};
      $scope.eventLogs = 'EV';
      $scope.systemLogs = 'S';
      $scope.agentLogs = 'E';
      $scope.taskLogs = 'T';
      $scope.allLogs = 'ALL';
      $scope.userTz = $window.userTz;
      $scope.jiraHost = $window.jiraHost;

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

      var isFinished = function(status) {
        switch (status) {
          case "success":
          return true;
          case "failed":
          return true;
          default:
          return false;
        }
      }

      $scope.getLogs = function() {
        $http.get('/json/task_log/' + $scope.taskId + '/' + $scope.task.execution + '?type=' + $scope.currentLogs).then(
          function(resp) {
            var taskScheduledStatus = "TASK_SCHEDULED";
            var data = resp.data;
            if ($scope.currentLogs == $scope.eventLogs) {
              // remove any scheduled events other than the most recent one
              var tmp = data.reverse();
              var foundScheduled = false;
              for (var i = tmp.length-1; i >=0; i--) {
                var event = tmp[i];
                if (foundScheduled && event.event_type === taskScheduledStatus) {
                  removeFromArray(tmp, i, i);
                  continue;
                }
                if (event.event_type === taskScheduledStatus) {
                  foundScheduled = true;
                }
              }
              $scope.eventLogData = tmp;
            } else {
              if (data && data.LogMessages) {
                //read the log messages out, and reverse their order (since they are returned backwards)
                $scope.logs = _.map(data.LogMessages, function(entry) {
                  var msg = entry.m.replace(/&#34;/g, '"');
                  var date = new Date(entry.ts);
                  return {
                    message: msg,
                    severity: entry.s,
                    timestamp: date,
                    version: entry.v
                  };
                });
              } else {
                $scope.logs = [];
              }
            }
          },
          function(resp) {
            notifier.pushNotification('Error retrieving logs: ' + resp.Data, 'errorHeader');
          });

          // If we already have an outstanding timeout, cancel it
          if ($scope.getLogsTimeout) {
            $timeout.cancel($scope.getLogsTimeout);
          }

          if (isFinished($scope.task.status)) {
            return;
          }

          $scope.getLogsTimeout = $timeout(function() {
            $scope.getLogs();
          }, 5000);
        };

        var removeFromArray = function(source, from, to) {
          var rest = source.slice((to || from) + 1 || source.length);
          source.length = from < 0 ? source.length + from : from;
          return source.push.apply(source, rest);
        };

        $scope.getRawLogLink = function(isRaw) {
          if ($scope.currentLogs === $scope.eventLogs) {
            return '/event_log/task/' + $scope.taskId;
          } else {
            var raw = isRaw ? '&text=true' : '';
            return '/task_log_raw/' + $scope.taskId + '/' + $scope.task.execution + '?type=' + $scope.currentLogs + raw;
          }
        };

        $scope.setTask = function(task) {
          $scope.task = task;
          $scope.taskId = task.id;
        };

        $scope.setTask($window.task_data);

      }]);
