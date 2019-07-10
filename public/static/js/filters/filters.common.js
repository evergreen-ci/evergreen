/**
 *  filters.common.js
 *
 *  Created on: September 12, 2013
 *      Author: Valeri Karpov
 *
 *  Common AngularJS filters that will be used on most pages
 *
 */

var filters = filters || {};

filters.common = angular.module('filters.common', []);

var LINK_REGEXP =
  /(http|https):\/\/([\w\-_]+(?:(?:\.[\w\-_]+)+))([\w\-\.,@?^=%&amp;:\/~\+#]*[\w\-\@?^=%&amp;\/~\+#])?/ig;

var JIRA_REGEXP = /[A-Z]{1,10}-\d{1,6}/ig;

var escapeHtml = function(str) {
  var div = document.createElement('div');
  div.appendChild(document.createTextNode(str));
  return div.innerHTML;
};


var isDismissed = function(bannerText) {
  return md5(bannerText) === localStorage.getItem("dismissed");
}

var bannerText = function() {
  return escapeHtml(window.BannerText);
}

var convertSingleTest = function(test, execution) {
  let output = {
    data: {
      "results": []
    }
  };
  output.create_time = test.created_at;
  if (test.info) {
    output.order = test.info.order;
    output.version_id = test.info.version;
    output.project_id = test.info.project;
    output.task_name = test.info.task_name;
    output.task_id = test.info.task_id;
    output.order = test.info.order;

    let versionParts = output.version_id.split("_");
    if (versionParts.length > 1) {
      output.revision = versionParts[versionParts.length - 1];
    }
  }
  if (execution && test.info.execution !== execution) {
    return output;
  }
  var result = {};
  var threads
  if (test.info && test.info.args) {
    threads = test.info.args.thread_level;
  } else {
    _.each(test.rollups.stats, function (stat) {
      if (stat.name === "WorkersMin") {
        threads = stat.val;
      }
    });
  }
  if (!threads) {
    return output;
  }
  result[threads] = {};

  _.each(test.rollups.stats, function (stat) {
      result[threads][stat.name] = Array.isArray(stat.val) ? stat.val[0].Value : stat.val;
      result[threads][stat.name + "_values"] = Array.isArray(stat.val) ? [stat.val[0].Value] : [stat.val];
  });
  output.data.results.push({
      "name": test.info.test_name,
      "isExpandedMetric": true,
      "results": result
  });
  
  return output;
}

filters.common.filter('conditional', function() {
  return function(b, t, f) {
    return b ? t : f;
  }
}).filter('min', function() {
  return function(v, m) {
    return Math.min(v, m);
  };
}).filter('default', function() {
  return function(input, def) {
    return input ? input : def;
  };
}).filter('undef', function() {
  return function(v) {
    return v == null || v == undefined;
  };
}).filter('shortenString', function() {
  // shortenString shortens a long string with the following options.
  // wordwise (boolean) - if true, cut only by words bounds
  // max (integer) - max length of the text, cut to this number of chars
  // tail (string, default: ' …') - add this string to the input string if the string was cut
  return function(value, wordwise, max, tail) {
    if (!value) {
        return '';
    }

    max = parseInt(max, 10);
    if (!max) {
        return value;
    }
    if (value.length <= max) {
        return value;
    }

    value = value.substr(0, max);
    if (wordwise) {
        var lastspace = value.lastIndexOf(' ');
        if (lastspace !== -1) {
            value = value.substr(0, lastspace);
        }
    }

    return value + (tail || ' …');
  };
}).filter('pluralize', function() {
  return function(v, noun) {
    if (isNaN(v)) {
      return noun + 's';
    } else if (v == 1) {
      return noun;
    } else {
      return noun + 's';
    }
  };
}).filter('capitalize', function() {
  return function(input) {
    if (!input) {
      return input;
    }

    var ret = "";
    var sp = input.split(' ');

    for (var i = 0; i < sp.length; ++i) {
      if (sp[i].charAt(0).toUpperCase() >= 'A' && sp[i].charAt(0).toUpperCase() <= 'Z') {
        sp[i] = sp[i].substr(0, 1).toUpperCase() + sp[i].substr(1);
      }
      ret += (i == 0 ? '' : ' ') + sp[i];
    }

    return ret;
  }
}).filter('dateFromNanoseconds', function() {
  return function(v) {
    return new Date(v / (1000 * 1000));
  }
}).filter('nanoToSeconds', function() {
  return function(nanoseconds) {
    return nanoseconds * 1000 * 1000 * 1000;
  }
}).filter('stringifyNanoseconds', function() {
  var NS_PER_MS = 1000 * 1000; // 10^6
  var NS_PER_SEC = NS_PER_MS * 1000
  var NS_PER_MINUTE = NS_PER_SEC * 60;
  var NS_PER_HOUR = NS_PER_MINUTE * 60;

  // stringifyNanoseconds takes an integer count of nanoseconds and
  // returns it formatted as a human readable string, like "1h32m40s"
  // If skipDayMax is true, then durations longer than 1 day will be represented
  // in hours. Otherwise, they will be displayed as '>=1 day'
  return function(input, skipDayMax, skipSecMax) {
    if (input === 0) {
      return "0 seconds";
    } else if (input == "unknown" || input < 0) {
      return "unknown";
    } else if (input < NS_PER_MS) {
      return "< 1 ms";
    } else if (input < NS_PER_SEC) {
      if (skipSecMax){
        return Math.floor(input / NS_PER_MS) + " ms";
      } else {
        return "< 1 second"
      }
    } else if (input < NS_PER_MINUTE) {
      return Math.floor(input / NS_PER_SEC) + " seconds";
    } else if (input < NS_PER_HOUR) {
      return Math.floor(input / NS_PER_MINUTE) + "m " + Math.floor((input % NS_PER_MINUTE) / NS_PER_SEC) + "s";
    } else if (input < NS_PER_HOUR * 24 || skipDayMax) {
      return Math.floor(input / NS_PER_HOUR) + "h " +
          Math.floor((input % NS_PER_HOUR) / NS_PER_MINUTE) + "m " +
          Math.floor((input % NS_PER_MINUTE) / NS_PER_SEC) + "s";
    } else {
      return ">= 1 day";
    }
  };
}).filter('linkify', function() {
  return function(input) {
    var ret = "";
    var index = 0;

    if (!input) {
      return input;
    }

    return input.replace(LINK_REGEXP, function(match) {
      return '<a href="' + match + '">' + match + '</a>';
    });
  };
}).filter('jiraLinkify', function($sce) {
  return function(input, jiraHost) {
    if (!input) {
      return input
    }
    return input.replace(JIRA_REGEXP, function(match) {
      return $sce.trustAsHtml(
        '<a href="https://'+jiraHost +'/browse/' + match + '">' + match + '</a>'
      )
    })
  }
}).filter('convertDateToUserTimezone', function() {
  return function(input, timezone, format) {
    return moment(input).tz(timezone).format(format);
  }
}).filter('encodeUri', function() {
  return function(url) {
    return encodeURIComponent(url);
  }
}).filter('ansi', function($window) {
  return function(input) {
    /* AnsiUp does some unexpected things in a pre tag, so run AnsiUp on a
     * line-by-line basis */
    return _.map(input.split('\n'), $window.ansi_up.ansi_to_html).join('\n');
  };
}).filter('trustAsHtml', function($sce) {
  return function(input) {
    return $sce.trustAsHtml(input);
  };
}).filter('escapeHtml', function() {
  return escapeHtml;
}).filter('pretty', function() {
  return function(obj) {
    return JSON.stringify(obj, null, 2);
  };
}).filter('ordinalNum', function() {
  // converts 1, 2, 3, etc. to 1st, 2nd, 3rd, etc.
  return function(n) {
    var s = ["th","st","nd","rd"], v = n % 100;
    // Past the first twenty, the first four of every ten have the special endings
    // Within in the first twenty, the first four have special endings
    // All of the others have the default ending
    return n + ( s[(v-20)%10] || s[v] || s[0] );
  }
}).filter('range', function() {
  // Returns an array of numbers which are >= low and < high. Useful for when
  // your use case doesn't quite fit into an `in` loop for ng-repeat
  return function(low, high) {
    var ret = [];
    for (var i = low; i < high; ++i) {
      ret.push(i);
    }
    return ret;
  };
}).filter('sample', function() {
  // Given a potentially long array, returns an array which contains equally
  // spaced elements from the original array such that there are no more than
  // `maxNum` elements. Effectively, gives you `maxNum` samples from the source
  // array.
  return function(arr, maxNum) {
    if (arr.length === 0) {
      return [];
    }
    if (arr.length < maxNum) {
      return arr;
    }

    var ret = [];
    var every = Math.floor(arr.length / maxNum);
    for (var i = 0; i < maxNum; ++i) {
      ret.push(arr[i * every]);
    }
    return ret;
  };
}).filter('fixedPrecisionTimeDiff', function() {
  var SECOND = 1000;
  var MINUTE = 60 * SECOND;
  var HOUR = 60 * MINUTE;
  var DAY = 24 * HOUR;

  return function(diff) {
    if (diff < SECOND) {
      // ex: 433ms
      return diff + 'ms';
    } else if (diff < MINUTE) {
      // ex: 17.2s
      return Math.floor(diff / SECOND) + '.' +
        Math.floor((diff % SECOND ) / 100) + 's'
    } else if (diff < HOUR) {
      // ex: 12m48s
      return Math.floor(diff / MINUTE) + 'm' +
        Math.floor((diff % MINUTE) / SECOND) + 's';
    } else if (diff < DAY) {
      // ex: 13h27m
      return Math.floor(diff / HOUR) + 'h' +
        Math.floor((diff % HOUR) / MINUTE) + 'm';
    } else {
      return '>=1d';
    }
  };
})
// Displays input value as percentage
// "0.85 | percentage" => "85"
.filter('percentage', function() {
  return function(input) {
    return input * 100
  }
})

.filter('statusFilter', function() {
  return function(task) {
    // for task test results, return the status passed in
    if (task !== Object(task)) {
      return task;
    }
    var cls = task.status;
    if (task.status == 'started') {
      cls = 'started';
    } else if (task.status == 'success') {
      cls = 'success';
    } else if (task.status == 'failed') {
      cls = 'failed';
      // Get a property name where task details are stored
      // Name may differe from API to API
      let detailsProp = _.find(['task_end_details', 'status_details'],  d => d in task)
      if (detailsProp) {
        let details = task[detailsProp]
        if ('type' in details) {
          if (details.type == 'system' && details.status != 'success') {
            cls = 'system-failed';
          }
          if (details.type == 'setup' && details.status != 'success') {
            cls = 'setup-failed';
          }
        }
        if ('timed_out' in details) {
          if (details.timed_out && 'desc' in details && details.desc == 'heartbeat') {
            cls = 'system-failed';
          }
        }
      }
    } else if (task.status == 'undispatched' || task.status == 'inactive' || task.status == 'unstarted' || (task.display_only && task.task_waiting)) {
        if (!task.activated) {
            cls = 'inactive';
        } else {
            cls = 'unstarted';
        }
    }
    return cls;
  }
})

.filter('statusLabel', function() {
  return function(task) {
    if (task.status == 'started') {
      return 'started';
    } else if (task.status == 'undispatched' && task.activated) {
      if (task.task_waiting) {
        return task.task_waiting;
      }
      return 'scheduled';
    } else if (task.status == 'undispatched' && !task.activated){
      // dispatch_time could be a string or a number. to check when the dispatch_time
      // is a real value, this if-statement accounts for cases where
      // dispatch_time is 0, "0" or even new Date(0) or older.
      if (!task.dispatch_time || task.dispatch_time == 0 || (typeof task.dispatch_time === "string" && +new Date(task.dispatch_time) <= 0)) {
        return "not scheduled"
      }
      return 'aborted';
    } else if (task.status == 'success') {
      return 'success';
    } else if (task.status == 'failed') {
      // Get a property name where task details are stored
      // Name may differe from API to API
      let detailsProp = _.find(['task_end_details', 'status_details'],  d => d in task)
      if (detailsProp) {
        let details = task[detailsProp]
        if ('timed_out' in details) {
          if (details.timed_out && 'desc' in details && details.desc == 'heartbeat') {
            return 'system unresponsive';
          }
          if (details.type == 'setup') {
            return 'setup timed out';
          }
          if (details.type == 'system') {
            return 'system timed out';
          }
          return 'test timed out';
        }
        if (details.type == 'system' && details.status != 'success') {
          return 'system failure';
        }
        if (details.type == 'setup' && details.status != 'success') {
          return 'setup failure';
        }
        return 'failed';
      }
    }
    if (task.task_waiting && !task.override_dependencies) {
        return task.task_waiting;
    }
    return task.status;
  }
})

.filter('endOfPath', function() {
  return function(input) {
    var lastSlash = input.lastIndexOf('/');
    if (lastSlash === -1 || lastSlash === input.length - 1) {
      // try to find the index using windows-style filesystem separators
      lastSlash = input.lastIndexOf('\\');
      if (lastSlash === -1 || lastSlash === input.length - 1) {
        return input;
      }
    }
    return input.substring(lastSlash + 1);
  }
})

.filter('buildStatus', function() {
  // given a list of tasks, returns the status of the overall build.
  return function(tasks) {
    var countSuccess = 0;
    var countInProgress = 0;
    var countActive = 0;
    for(i=0;i<tasks.length;i++){
      // if any task is failed, the build status is "failed"
      if(tasks[i].status == "failed"){
        return "block-status-failed";
      }
      if(tasks[i].status == "success"){
        countSuccess++;
      } else if(tasks[i].status == "dispatched" || tasks[i].status=="started"){
        countInProgress++;
      } else if(tasks[i].status == "undispatched") {
        countActive += tasks[i].activated ? 1 : 0;
      }
    }
    if(countSuccess == tasks.length){
      // all tasks are passing
      return "block-status-success";
    }else if(countInProgress>0){
      // no failures yet, but at least 1 task in still progress
      return "block-status-started";
    }else if(countActive>0){
      // no failures yet, but at least 1 task still active
      return "block-status-created";
    }
    // no active tasks pending
    return "block-status-inactive";
  }
})
.filter('expandedMetricConverter', function() {
  return function(data, execution) {
    if (!data) {
      return null;
    }
    var output = {
      data: {
          "results": []
      }
    };

    _.each(data, function(test) {
      let singleTest = convertSingleTest(test, execution);
      output.data.results = output.data.results.concat(singleTest.data.results);
    })

    return output;
  }
})
.filter('expandedHistoryConverter', function() {
  return function(data, execution) {
    if (!data) {
      return null;
    }
    let output = [];

    _.each(data, function(test) {
      output.push(convertSingleTest(test, execution));
    })

    return output;
  }
})
// merges two sets of perf results, taking metadata from the second sample but giving test result preference to the first one
.filter('mergePerfResults', function() {
  return function(firstSample, secondSample) {
    if (!firstSample) {
      return secondSample;
    }
    if (!secondSample) {
      return firstSample;
    }
    var toReturn = Object.assign({}, secondSample);
    tempResults = {};

    _.each(secondSample.data.results, function(result) {
      tempResults[result.name] = result;
    })
    _.each(firstSample.data.results, function(result) {
      tempResults[result.name] = result;
    })
    toReturn.data.results = _.toArray(tempResults);

    return toReturn;
  }
})
