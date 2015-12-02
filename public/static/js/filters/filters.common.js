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
    if (input == 0) {
      return "Not Started";
    } else if (input < NS_PER_MS) {
      return "< 1 ms";
    } else if (input < NS_PER_SEC) {
      if (skipSecMax){
        return Math.floor(input / NS_PER_MS) + " ms";
      }
        else {
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
    } else if (input == "unknown") {
      return "unknown";
    }  else {
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
});
