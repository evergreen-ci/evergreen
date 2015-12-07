/**
 *  filters.capitalize.test.js
 *
 *  Created on: September 12, 2013
 *      Author: Valeri Karpov
 *
 *  Karma-based unit tests for the capitalize filter, which should convert the first letter of
 *  each word in the input to upper case.
 *  (see public/static/js/tests/conf/karma.conf.js)
 *
 */

describe('capitalize', function() {
  beforeEach(module('filters.common'));

  it("should do nothing for a string which has no words", inject(function($filter) {
    expect( $filter('capitalize')("37signals") ).toBe("37signals");
  }));

  it("should convert a single word to upper case", inject(function($filter) {
    expect( $filter('capitalize')("unstarted") ).
        toBe("Unstarted");
  }));

  it("should avoid isolated numbers", inject(function($filter) {
    expect( $filter('capitalize')("2 chainz") ).
        toBe("2 Chainz");
  }));

  it("should handle multiple words", inject(function($filter) {
    expect( $filter('capitalize')("I ate 2 apples") ).
        toBe("I Ate 2 Apples");
  }));
});
