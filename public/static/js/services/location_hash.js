var mciServices = mciServices || {};

mciServices.locationHash = angular.module('mciServices.locationHash', []);

/* Wrapper around location hash for MCI. Angular's $location has some
 * weirdnesses and doesn't map quite well to how we use hashes. */
mciServices.locationHash.factory('$locationHash', function($window) {
  var locationHash = {
    get : function() {
      var hash = $window.location.hash.substr(1); // Get rid of leading '#'
      if (hash.charAt(0) == '/') {
        hash = hash.substr(1);
      }

      // If hash doesn't have any key-value pairs, return empty
      if (hash.length == 0 || hash.indexOf('=') == -1) {
        return {};
      }

      // First split by '&', then by '=', then decodeURI on each resulting
      // string
      var keyValuePairs = _.map(hash.split('&'), function(str) {
        var pair = _.map(str.split('='), decodeURIComponent);
        return pair;
      });

      var ret = {};
      _.each(keyValuePairs, function(pair) {
        ret[pair[0]] = pair[1];
      });

      return ret;
    },
    set : function(obj) {
      var str = '';
      // { a : 1, b : 'hello world' } => "a=1&b=hello%20world"
      _.each(obj, function(value, key) {
        if (str.length > 0) {
          str += '&';
        }

        str += encodeURIComponent(key) + '=' + encodeURIComponent(value);
      });

      $window.location.hash = str;
    }
  };

  return locationHash;
});