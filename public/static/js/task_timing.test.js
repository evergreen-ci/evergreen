describe('TaskTimingControllerTest', function() {
  beforeEach(module('MCI'));

  var ctrl, scope

  const $window = {
    activeProject: {
      task_names: ['T_A', 'T_C', 'T_D'],
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
      }, {
        name: 'BV_C',
        display_tasks: [{
          name: 'T_X',
          execution_tasks: ['T_C']
        }]
      }, {
        name: 'BV_D',
        display_tasks: [{
          name: 'T_X',
          execution_tasks: ['T_C']
        }]
      }]
    }
  }

  beforeEach(inject(function($injector, $rootScope, _$controller_) {
    // Controller's code mutates input data
    // makie a hard copy of some data parts
    let windowCopy = {activeProject: _.clone($window.activeProject)}
    windowCopy.activeProject.task_names = _.map(
      $window.activeProject.task_names
    )

    let $controller = _$controller_
    scope = $rootScope.$new()
    ctrl = $controller('TaskTimingController', {
      $scope: scope,
      $window: windowCopy,
    })
  }))

  it('reads window.activeProject', function() {
    expect(scope.selectableTasksPerBV).toEqual({
      BV_A: ['T_A', 'T_B'],
      BV_B: ['T_A', 'T_D'],
      BV_C: ['T_X'],
      BV_D: ['T_X'],
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

  // Execution tasks shouldn't be displayed - EVG-5608
  it('collect list of unique display task names across al bvs', function() {
    expect(scope.taskNames.length).toEqual(5)
    expect(scope.taskNames).toContain('All Tasks')
    expect(scope.taskNames).toContain('T_A')
    expect(scope.taskNames).toContain('T_B')
    expect(scope.taskNames).toContain('T_D')
    expect(scope.taskNames).toContain('T_X')
  })
})
