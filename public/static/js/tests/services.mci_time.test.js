/**
 *  services.mci_time.test.js
 *
 *  Created on: September 26, 2013
 *      Author: Valeri Karpov
 *
 *  Karma-based unit tests for mciTime service (see public/static/js/tests/conf/karma.conf.js)
 *
 */

describe('mciTime', function() {
  beforeEach(module('MCI'));

  var dateFromMillis = function(ms) {
    return new Date(ms);
  }

  it("should return 0 for undefined parameters", inject(function(mciTime) {
    expect(mciTime.finishConditional()).toBe(0);
  }));

  it("should return 0 for invalid start", inject(function(mciTime) {
    expect(mciTime.finishConditional("")).toBe(0);
  }));

  it("should return difference between start and now if finish is undefined", inject(function(mciTime) {
    mciTime.now = function() {
      return new Date(26);
    };

    expect(mciTime.finishConditional(dateFromMillis(1))).toBe(25);
  }));

  it("should return difference between start and finish when both are defined", inject(function(mciTime) {
    mciTime.now = function() {
      return null;
    };

    expect(mciTime.finishConditional(dateFromMillis(1), dateFromMillis(51))).toBe(50);
  }));
});
