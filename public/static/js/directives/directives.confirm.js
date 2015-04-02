var directives = directives || {};

directives.confirm = angular.module('directives.confirm', []);

directives.confirm.directive('confirmOnExit', function() {
    return {
        scope: {
            confirmOnExit: '&',
            confirmMessageWindow: '@',
            confirmMessageRoute: '@',
            confirmMessage: '@'
        },
        link: function($scope, elem, attrs) {
            window.onbeforeunload = function(){
                if ($scope.confirmOnExit()) {
                    return $scope.confirmMessageWindow || $scope.confirmMessage;
                }
            }
            var $locationChangeStartUnbind = $scope.$on('$locationChangeStart', function(event, next, current) {
                if ($scope.confirmOnExit()) {
                    if(! confirm($scope.confirmMessageRoute || $scope.confirmMessage)) {
                        event.preventDefault();
                    }
                }
            });

            $scope.$on('$destroy', function() {
                window.onbeforeunload = null;
                $locationChangeStartUnbind();
            });
        }
    };
});
