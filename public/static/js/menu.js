function setBanner() {
  var theme = window.BannerTheme;
  $("#bannerText").html(window.BannerText);
  switch(theme) {
    case "important":
      $("#bannerIcon").append("<i class='fa fa-exclamation'></i>");
      $("#bannerIcon").addClass("banner-icon-important");
      $("#bannerBack").addClass("banner-text-important");
      break;
    case "warning":
      $("#bannerIcon").append("<i class='fa fa-exclamation-triangle'></i>");
      $("#bannerIcon").addClass("banner-icon-warning");
      $("#bannerBack").addClass("banner-text-warning");
      break;
    case "information":
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
