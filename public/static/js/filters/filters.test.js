describe('FiltersTest', function() {
  beforeEach(module('MCI'));

  let statusLabel

  beforeEach(inject(function($injector) {
    let $filter = $injector.get('$filter')
    statusLabel = $filter('statusLabel')
  }))


  describe('statusLabel filter test', function() {
    it('Tasks with overriden dependencies are not blocked', function() {
      expect(
        statusLabel({
          task_waiting: 'blocked',
          override_dependencies: true,
          status: 'success'
        })
      ).toBe('success')
    })

    it('Successfull tasks displayed as successfull', function() {
      expect(
        statusLabel({status: 'success'})
      ).toBe('success')
    })

    it('Waiting tasks displayed as blocked', function() {
      expect(
        statusLabel({task_waiting: 'blocked'})
      ).toBe('blocked')
    })
  })
})
