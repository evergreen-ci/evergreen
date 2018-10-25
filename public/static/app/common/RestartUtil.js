mciModule.factory('RestartUtil', function($filter) {
  return {
    STATUS: {
      ALL: {
        name: 'All',
        matches: _.constant(true),
      },
      NONE: {
        name: 'None',
        matches: _.constant(false),
      },
      FAILURES: {
        name: 'Failures',
        matches: function(task) { return task.status == 'failed' }
      },
      SYSTEM_FAILURES: {
        name: 'System Failures',
        matches: function(task) {
          return $filter('statusFilter')(task) == 'system-failed'
        }
      },
      SETUP_FAILURES: {
        name: 'Setup Failures',
        matches: function(task) {
          return $filter('statusFilter')(task) == 'setup-failed'
        }
      },
    }
  }
})
