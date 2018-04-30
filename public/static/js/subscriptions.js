function addSubscriber($mdDialog, triggers, callback) {
    return subscriberPromise($mdDialog, "Add", triggers, callback)
}

function subscriberPromise($mdDialog, verb, triggers, callback) {
    return $mdDialog.alert({
        title:"test",
        templateUrl: "static/partials/subscription_modal.html",
        controller: subCtrl,
        locals: {
            triggers: triggers,
            verb: verb,
            callback: callback
        },
    });
}

function subCtrl($scope, $mdDialog, verb, triggers, callback) {
    // labels should complete the following sentence fragment:
    // 'then notify by ...'
    $scope.subscriber = {};
    $scope.subscription_methods = [
        {
            value: "email",
            label: "sending an email",
        },
        {
            value: "slack",
            label: "sending a slack message",
        },
        {
            value: "jira-comment",
            label: "making a comment on a JIRA issue",
        },
        {
            value: "jira-issue",
            label: "making a JIRA issue",
        },
        {
            value: "evergreen-webhook",
            label: "posting to an external server",
        },
        // Github status api is deliberately omitted here
    ];
    $scope.triggers = triggers;
    $scope.verb = verb;
    $scope.closeDialog = function(save) {
        if(save === true) {
            subscriber = {
                type: $scope.method.value,
                target: $scope.targets[$scope.method.value],
            }
            callback(subscriber);
        }
        $mdDialog.hide();
    };

    $scope.generateSecret = function() {
        var text = "";
        var possible = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";

        for (var i = 0; i < 64; i++)
            text += possible.charAt(Math.floor(Math.random() * possible.length));

        return text;
    };

    $scope.valid = function() {
        if ($scope.trigger == null ) {
            return false;
        }

        if ($scope.method.value === 'jira-comment') {
            return $scope.targets['jira-comment'].match(".+-[0-9]+") !== null

        }else if ($scope.method.value === 'jira-issue') {
            return $scope.targets['jira-issue'].match(".+") !== null

        }else if ($scope.method.value === 'slack') {
            return $scope.targets['slack'].match("(#|@).+") !== null

        }else if ($scope.method.value === 'email') {
            return $scope.targets['email'].match(".+@.+") !== null

        }else if ($scope.method.value === 'evergreen-webhook') {
            if ($scope.targets['evergreen-webhook'] == undefined) {
                return false;
            }
            return ($scope.targets['evergreen-webhook'].secret.length >= 32 &&
                $scope.targets['evergreen-webhook'].url.match("https://.+") !== null)
        }

        return false;
    };
    $scope.method = {};
    $scope.targets = {
        'evergreen-webhook': {
            secret: $scope.generateSecret(),
        },
    };
}
