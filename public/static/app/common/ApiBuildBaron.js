mciModule.factory('ApiBuildBaron', function($http, ApiUtil, API_BUILD_BARON) {
  var get = ApiUtil.httpGetter(API_BUILD_BARON.BASE)

  return {
    getTicketsByTaskId: function(taskId) {
      return get(API_BUILD_BARON.TICKETS_BY_TASK_ID, {task_id: taskId})
    },
  }
})
