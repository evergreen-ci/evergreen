var directives = directives || {};

directives.admin = angular.module('directives.admin', []);

directives.admin.directive('adminModal', function() {
    return {
        restrict: 'E',
        transclude: true,
        templateUrl: '/static/partials/admin_modal_base.html'
    }
});

directives.admin.directive('alertModal', function() {
    return {
        restrict: 'E',
        transclude: true,
        templateUrl: '/static/partials/alert_modal.html'
    }
});
