function setBanner() {
  var theme = window.BannerTheme;
  var text = bannerText();
  var jiraLinkify = angular.element(document).injector().get("$filter")(
    "jiraLinkify"
  );
  if (isDismissed(text)) {
    $("#banner-container").addClass("nodisp");
    return;
  }
  $("#bannerText").html(jiraLinkify(text, window.JiraHost));
  switch (theme) {
    case "important":
    case "IMPORTANT":
      $("#bannerIcon").append("<i class='fa fa-exclamation'></i>");
      $("#bannerIcon").addClass("banner-icon-important");
      $("#bannerBack").addClass("banner-text-important");
      break;
    case "warning":
    case "WARNING":
      $("#bannerIcon").append("<i class='fa fa-exclamation-triangle'></i>");
      $("#bannerIcon").addClass("banner-icon-warning");
      $("#bannerBack").addClass("banner-text-warning");
      break;
    case "information":
    case "INFORMATION":
      $("#bannerIcon").append("<i class='fa fa-info-circle'></i>");
      $("#bannerIcon").addClass("banner-icon-information");
      $("#bannerBack").addClass("banner-text-information");
      break;
    default:
      $("#bannerIcon").append("<i class='fa fa-bullhorn'></i>");
      $("#bannerIcon").addClass("banner-icon-announcement");
      $("#bannerBack").addClass("banner-text-announcement");
  }
}

function dismissBanner() {
  localStorage.setItem("dismissed", md5(bannerText()));
  $("#banner-container").addClass("nodisp");
}
mciModule.controller("NavBarController", function (
  $scope,
  $http,
  $timeout,
  $rootScope,
  mciTime,
  $window
) {
  $scope.newUILink = $window.NewUILink;
  $scope.showNewUILink = $window.user && $window.NewUILink;
});
