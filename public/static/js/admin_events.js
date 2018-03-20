mciModule.controller('AdminEventsController', ['$scope','$window', 'mciAdminRestService', 'notificationService', function($scope, $window, mciAdminRestService, notificationService) {
  $scope.load = function() {
    $scope.userTz = $window.userTz;
    $scope.Events = [];
    // hook up incremental loading when scrolling to bottom
    $(window).scroll(function() {
      if ($(window).scrollTop() + $(window).height() == $(document).height()) {
        if ($scope.nextTs !== "") {
          $scope.getMoreEvents($scope.nextTs);
        }
      }
    });
    $scope.getMoreEvents();
  }

  $scope.getMoreEvents = function(timestamp) {
    var successHandler = function(resp) {
      for (var i = 0; i < resp.data.length; i ++) {
        event = resp.data[i];
        diff = DeepDiff(event.before, event.after);
        event.diff = (diff) ? diff : [];
        for (var j = 0; j < event.diff.length; j++) {
          eventLine = event.diff[j]
          eventLine.text = getDiffText(eventLine);
        }
      }
      $scope.Events = $scope.Events.concat(resp.data);
      $scope.nextTs = getNextTs(resp.headers().link);
    }
    var errorHandler = function(resp) {
      notificationService.pushNotification("Error loading events: " + resp.data.error, "errorHeader");
    }
    mciAdminRestService.getEvents(timestamp, 0, { success: successHandler, error: errorHandler });
  }

  $scope.revertEvent = function(guid) {
    var successHandler = function(resp) {
      window.location.href = "/admin/events";
    }
    var errorHandler = function(resp) {
      notificationService.pushNotification("Error reverting settings: " + resp.data.error, "errorHeader");
    }
    mciAdminRestService.revertEvent(guid, { success: successHandler, error: errorHandler });
  }

  getNextTs = function(pageLink) {
    if (!pageLink) {
      return "";
    }
    nextLink = pageLink.substr(1).split(">")[0];
    if (nextLink === "") {
      return "";
    }
    return getQueryParam("ts", nextLink);
  }

  getDiffText = function(diff) {
    //see https://github.com/flitbit/diff for the format of the diff object
    var property = "";
    for (var i = 0; i < diff.path.length; i++) {
      name = diff.path[i];
      if (typeof name === "number") {
        property += "[" + name + "]";
      } else {
        if (i !== 0) {
          property += ".";
        }
        property += name;
      }
    }
    if (diff.index) {
      property += "[" + diff.index + "]";
    }
    var before = "";
    switch (diff.kind) {
      case "N":
      before = "";
      break;
      case "D":
      case "E":
      before = diff.lhs;
      break;
      case "A":
      if (diff.item.lhs) {
        before = diff.item.lhs;
      }
      break;
      default:
      before = "Unknown value";
    }
    var after = "";
    switch (diff.kind) {
      case "N":
      case "E":
      after = diff.rhs;
      break;
      case "D":
      after = "";
      break;
      case "A":
      if (diff.item.rhs) {
        after = diff.item.rhs;
      }
      break;
      default:
      after = "Unknown value";
    }

    return {"property": property, "before": before, "after": after};
  }

  getQueryParam = function(name, url) {
    if (!url) { return ""; }
    name = name.replace(/[\[\]]/g, "\\$&");
    var regex = new RegExp("[?&]" + name + "(=([^&#]*)|&|#|$)"),
    results = regex.exec(url);
    if (!results) { return null; }
    if (!results[2]) { return ""; }
    return decodeURIComponent(results[2].replace(/\+/g, " "));
  }

  $scope.load();
}]);
