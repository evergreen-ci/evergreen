/**
 *  stringify_nanoseconds.test.js
 *
 *  Created on: September 10, 2013
 *      Author: Valeri Karpov
 *
 *  Karma-based unit tests for stringifyNanoseconds filter (see public/static/js/tests/conf/karma.conf.js)
 *
 */

describe('stringifyNanoseconds', function() {
  beforeEach(module('filters.common'));

  var NS_PER_SEC = 1000 * 1000 * 1000;
  var NS_PER_MINUTE = NS_PER_SEC * 60;
  var NS_PER_HOUR = NS_PER_MINUTE * 60;

  it("should return the default value for zero", inject(function($filter) {
    expect($filter('stringifyNanoseconds')(0)).toBe("Not Started");
  }));

  it("should return default value for small inputs", inject(function($filter) {
    expect($filter('stringifyNanoseconds')(NS_PER_SEC * 0.5)).toBe("< 1 second");
  }));

  it("should return number of seconds for value < 1 minute", inject(function($filter) {
    expect($filter('stringifyNanoseconds')(NS_PER_SEC * 5)).toBe("5 seconds");
  }));

  it("should return minutes and seconds for value < 1 hour", inject(function($filter) {
    expect($filter('stringifyNanoseconds')(NS_PER_MINUTE * 2 + NS_PER_SEC * 15)).toBe("2m 15s");
  }));

  it("should return minutes, hours, and seconds for value < 24 hours", inject(function($filter) {
    expect($filter('stringifyNanoseconds')(NS_PER_HOUR * 12 + NS_PER_MINUTE * 3 + NS_PER_SEC * 5)).toBe("12h 3m 5s");
  }));

  it("should return default value for value >= 24 hours", inject(function($filter) {
    expect($filter('stringifyNanoseconds')(NS_PER_HOUR * 36)).toBe(">= 1 day");
  }));
});
