describe('HazardLevelCellTest', () => {
  beforeEach(module('MCI'));

  var $compile, $rootScope;

  beforeEach(inject(function(_$compile_, _$rootScope_){
    // The injector unwraps the underscores (_) from around the parameter names when matching
    $compile = _$compile_;
    $rootScope = _$rootScope_;
  }));

  it('should handle a simple decrease', () => {
    var row = {
      entity: {
        statistics: {
          previous: {
            mean: 10
          },
          next: {
            mean: 5,
          },
        }
      }
    }
    row = JSON.stringify(row);
    var element = $compile(`<hazard-level-cell row=${row} />`)($rootScope);
    $rootScope.$digest();

    var expectedResult = '-50%';
    expect(element.text()).toBe(expectedResult);
  });


  it('should handle a simple increase', () => {
    var row = {
      entity: {
        statistics: {
          previous: {
            mean: 5
          },
          next: {
            mean: 10,
          },
        }
      }
    }
    row = JSON.stringify(row);
    var element = $compile(`<hazard-level-cell row=${row} />`)($rootScope);
    $rootScope.$digest();

    var expectedResult = '+100%';
    expect(element.text()).toBe(expectedResult);
  });

  it('should handle a latency increase case', () => {
    var row = {
      entity: {
        statistics: {
          previous: {
            mean: -10,
          },
          next: {
            mean: -100,
          },
        }
      }
    }
    row = JSON.stringify(row);
    var element = $compile(`<hazard-level-cell row=${row} />`)($rootScope);
    $rootScope.$digest();

    var expectedResult = '-900%';
    expect(element.text()).toBe(expectedResult);
  });

  it('should handle a latency decrease case', () => {
    var row = {
      entity: {
        statistics: {
          previous: {
            mean: -100,
          },
          next: {
            mean: -10,
          },
        }
      }
    }
    row = JSON.stringify(row);
    var element = $compile(`<hazard-level-cell row=${row} />`)($rootScope);
    $rootScope.$digest();

    var expectedResult = '+90%';
    expect(element.text()).toBe(expectedResult);
  });

})