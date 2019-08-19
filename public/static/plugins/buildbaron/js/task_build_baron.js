mciModule.controller('TaskBuildBaronCtrl', function($scope, $http, $window, $interval) {
  $scope.conf = $window.plugins["buildbaron"];

  var statusKeys = {
    "Blocked": 1,
    "Open": 1,
    "In Progress": 1,
    "Waiting for bug fix": 1,
    "Closed": 2,
    "Resolved": 2,
  };

  $scope.getBuildBaronResults = function() {
    $http.get('/plugin/buildbaron/jira_bf_search/' + $scope.taskId + '/' + $scope.taskExec).then(
      function(resp) {
        var issues = resp.data.issues;
        var searchString = resp.data.search;
        $scope.source = resp.data.source;
        $scope.enableFeedback = ($scope.source === "BF Suggestion Server");
        $scope.JiraLink = getJqlUrl(searchString);
        $scope.featuresLink = resp.data.features_url;
        if (issues && issues.length > 0 ) {
          // we must sort with native js, since Angular does not
          // allow us to use conditionals when comparing two entries.
          _.each(issues, function(i){i.statusKey = statusKeys[i.fields.status.name];});
          issues.sort(function(a,b){
            // first, sort [Open, In Progress, Blocked] ahead of [Closed, Resolved];
            // if the statuses are in the same group, we move on to additional criteria.
            if (a.statusKey != b.statusKey) {
              return a.statusKey - b.statusKey;
            }
            // next, prefer tickets resolved with "Fixed"
            if (a.statusKey == 2) {
              // The alternative suggestion endpoint may not return the resolution of tickets, even
              // if their status is "Fixed" or "Closed". We fall back to comparing the tickets by
              // their updated date.
              a.fields.resolution = a.fields.resolution || {};
              b.fields.resolution = b.fields.resolution || {};

              if (a.fields.resolution.name != b.fields.resolution.name) {
                if (a.fields.resolution.name == "Fixed") {
                    return -1;
                }
                if (b.fields.resolution.name == "Fixed") {
                    return 1;
                }
              }
            }
            // finally, order by latest update time
            return b.fields.updated.localeCompare(a.fields.updated);
          });
          $scope.build_baron_results = issues;
          $scope.build_baron_status = "success";
        } else {
          $scope.build_baron_status = "nothing";
        }
      },
    function(resp) {
      $scope.build_baron_status = "error";
    });
  };

  $scope.getCreatedTickets = function() {
    $http.get('/plugin/buildbaron/created_tickets/' + $scope.taskId).then(
      function(resp) {
        $scope.created_tickets = resp.data;
      });
  }

  $scope.getNote = function() {
    $http.get('/plugin/buildbaron/note/' + $scope.taskId ).then(
      function(resp) {
        var data = resp.data;
        // the GET can return null, for empty notes
        if (data) {
          $scope.editTime = data.time;
          if (data.content) {
            $scope.note = data.content;
          }
        }
        $scope.loaded = true;
      },
    function(resp) {
      $scope.build_baron_status = "error";
      $scope.loaded = true;
    });
  };

  $scope.saveNote = _.debounce(function() {
    // we attach the previous editTime to ensure we
    // don't overwrite more recent edits the user
    // might have missed
    $http.put('/plugin/buildbaron/note/' + $scope.taskId,
        {content: $scope.note, time: $scope.editTime}).then(
      function(resp) {
        var data = resp.data;
        $scope.editTime = data.time;
        $scope.editing = false;
      },
    function(resp) {
      var jqXHR = resp.data;
      var err = "error saving note";
      if (jqXHR) {
        // append an error message if we get one
        err += ": " + jqXHR;
      }
      alert(err);
    });
  });

  $scope.fileTicket = _.debounce(function(){
    $scope.creatingTicket = true;
    $http.post('/plugin/buildbaron/file_ticket',
        {task: $scope.taskId, tests: $scope.ticketTests}).then(
      function(resp) {
        $scope.creatingTicket = false;
        $scope.createdTicket = true;
        $scope.getTickets = $interval($scope.getCreatedTickets, 1000);
      },
    function(resp) {
      var jqXHR = resp.data;
      var err = "error filing ticket";
      if (jqXHR) {
        // append an error message if we get one
        err += ": " + jqXHR;
      }
      alert(err);
      $scope.creatingTicket = false;
    });
  });

  $scope.getFeedback = function(type, data) {
    $http.get('/plugin/buildbaron/feedback/' + $scope.taskId + '/' + $scope.taskExec).then(
      // Success
      function(resp) {
        resp.data.forEach(function (item) {
          $scope.feedback[item.type] = item.data;
          if (item.type == "suggestions_quality" && item.data.comment) {
            $scope.feedbackComment = item.data.comment;
          }
        });
      })

  }

  $scope.toggleFeedback = function(type, data) {
    var feedback_value = $scope.feedback[type];
    if (type === "time_spent") {
      if (feedback_value.time_spent === data.time_spent) {
          $scope.removeFeedback(type);
      } else {
          $scope.sendFeedback(type, data);
      }
    } else if (type === "suggestions_quality") {
      if (feedback_value.ok === data.ok) {
          $scope.removeFeedback(type);
      } else if (data.ok) {
          $scope.sendFeedback(type, data);
      } else {
          // We are showing the comment field instead of sending the feedback directly.
          $scope.showComment = true;
      }
    }
  }

  $scope.sendFeedback = function(type, data) {
    $http.post('/plugin/buildbaron/feedback',
               {type: type,
                data:  data,
                task_id: $scope.taskId,
                execution: $scope.taskExec}).then(
      // Success
      function(resp) {
          $scope.feedback[type] = data;
      })
  }

  $scope.removeFeedback = function(type) {
    $http.delete('/plugin/buildbaron/feedback/' + $scope.taskId + '/' + $scope.taskExec + '/' + type).then(
      // Success
      function(resp) {
          $scope.feedback[type] = {};
      })
  }

  $scope.clearFeedbackDescriptionIfNotClicked = function() {
    if (!$scope.showComment) {
      $scope.feedbackDescription = "";
    }
  }

  $scope.loaded = false;
  $scope.have_user = $window.have_user;
  $scope.editing = false;
  $scope.editTime = 0;
  $scope.note = "";
  $scope.enableFeedback = false;
  $scope.feedback = {time_spent: {}, suggestions_quality: {}};
  $scope.feedbackDescription = "";
  $scope.showComment = false;


  $scope.newTicket = false;
  $scope.ticketTests = [];
  $scope.creatingTicket = false;

  $scope.setTask = function(task) {
    $scope.task = task;
    $scope.taskId = task.id;
    $scope.taskExec = task.execution;
    $scope.failed = _.filter(task.test_results, function(test){return test.status == 'fail'});
    // special case where we don't need user input when there is only one failure
    if ($scope.failed.length == 1) {
      $scope.ticketTests = [$scope.failed[0].test_file];
    }
  };

  $scope.setTask($window.task_data);
  if ( $scope.conf.enabled && $scope.have_user && $scope.task.status == "failed" ) {
    $scope.build_baron_status = "loading";
    $scope.getBuildBaronResults();
    $scope.getFeedback();
  }
  if($scope.conf.enabled && $scope.have_user){
    $scope.getNote();
  }
  $scope.getCreatedTickets();

  $scope.stopGetTickets = function() {
    if (angular.isDefined($scope.getTickets)) {
      $interval.cancel($scope.getTickets);
      $scope.getTickets = undefined;
    }
  }

  $scope.$on("$destroy", function() {
    $scope.stopGetTickets();
  });

  $scope.clearTicket = function(){
    $scope.newTicket = true;
    $scope.ticketTests = [];
    $scope.setTask($window.task_data);
  }

  var getJqlUrl = function(jql) {
    jqlEscaped = encodeURIComponent(jql);
    return 'https://jira.mongodb.org/secure/IssueNavigator.jspa?jql=' + jqlEscaped;
  }

});
