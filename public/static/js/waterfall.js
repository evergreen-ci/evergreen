mciModule.controller("WaterfallController", function(notificationService, mciSubscriptionsService, $mdDialog, $mdToast) {
  const angular = {
    notificationService,
    mciSubscriptionsService,
    $mdToast,
    $mdDialog
  };

  window.waterfallInstance.injectAngular(angular);
});
