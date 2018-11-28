describe('TaskTimingControllerTest', function() {
  beforeEach(module('MCI'));

  var ctrl, scope

  const $window = {
    activeProject: {
      task_names: ['T_A', 'T_B', 'T_C', 'T_D'],
      build_variants: [{
        name: 'BV_A',
        task_names: ['T_A', 'T_C'],
        display_tasks: [{
          name: 'T_B',
          execution_tasks: ['T_C']
        }]
      }, {
        name: 'BV_B',
        task_names: ['T_A', 'T_D'],
        display_tasks: [],
      }]
    }
  }

  beforeEach(inject(function($injector, $rootScope, _$controller_) {
    let $controller = _$controller_
    scope = $rootScope.$new()
    ctrl = $controller('TaskTimingController', {
      $scope: scope,
      $window: $window,
    })
  }))

  it('reads window.activeProject', function() {
    expect(scope.selectableTasksPerBV).toEqual({
      BV_A: ['T_A', 'T_B'],
      BV_B: ['T_A', 'T_D'],
    })
  })

  it('checks task could be selected', function() {
    const expectations = {
      BV_A: {
        T_A: true,
        T_B: true,
        T_C: false,
        T_D: false,
        'All Tasks': true,
      },
      BV_B: {
        T_A: true,
        T_B: false,
        T_C: false,
        T_D: true,
        'All Tasks': true,
      },
    }

    _.each(expectations, function(taskExpectations, bv) {
      scope.setBuildVariant(
        _.findWhere($window.activeProject.build_variants, {name: bv})
      )
      
      _.each(taskExpectations, function(checkStatus, task) {
        expect(
          scope.checkTaskForGraph(task)
        ).toBe(checkStatus)
      })
    })
  })
})
