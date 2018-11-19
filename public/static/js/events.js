mciModule.factory('eventsService', ['notificationService', function(notificationService) {
  var getMoreEvents = function(scope, mciRestService, resourceId) {
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
        scope.Events = scope.Events.concat(resp.data);
        scope.nextTs = getNextTs(resp.headers().link);
      }
      var errorHandler = function(resp) {
         notificationService.pushNotification("Error loading events: " + resp.data.error, "errorHeader");
      }

      mciRestService.getEvents(scope.nextTs, 0, { success: successHandler, error: errorHandler }, resourceId);
    }

  var getNextTs = function(pageLink) {
    if (!pageLink) {
      return "";
    }
    nextLink = pageLink.substr(1).split(">")[0];
    if (nextLink === "") {
      return "";
    }
    return getQueryParam("ts", nextLink);
  }

  var getDiffText = function(diff) {
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

  var getQueryParam = function(name, url) {
    if (!url) { return ""; }
    name = name.replace(/[\[\]]/g, "\\$&");
    var regex = new RegExp("[?&]" + name + "(=([^&#]*)|&|#|$)"),
    results = regex.exec(url);
    if (!results) { return null; }
    if (!results[2]) { return ""; }
    return decodeURIComponent(results[2].replace(/\+/g, " "));
  }

  return {
    getMoreEvents: getMoreEvents,
  }
}]);
