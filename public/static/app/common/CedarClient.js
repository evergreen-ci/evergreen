mciModule.factory('CedarClient', function ($http, $filter, ApiUtil, CEDAR_API, CEDAR_APP_URL) {
  const cedarAPI = ApiUtil.httpGetter(CEDAR_APP_URL + CEDAR_API.BASE);

  return {
    getVersionChangePoints: function (projectId, page, pageSize) {
      return cedarAPI(CEDAR_API.CHANGE_POINTS_BY_VERSION, {
        projectId: projectId,
      }, {
        page: page,
        pageSize: pageSize
      })
    },
  }
});