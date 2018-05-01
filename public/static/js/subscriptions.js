function subscriberLabel(subscriber) {
    if (subscriber.type === 'jira-comment') {
        return "Post a comment on JIRA issue " + subscriber.target;

    }else if (subscriber.type === 'jira-issue') {
        return "Create a JIRA issue in " + subscriber.target;

    }else if (subscriber.type === 'slack') {
        return "Send a slack message to " + subscriber.target;

    }else if (subscriber.type === 'email') {
        return "Send an email to " + subscriber.target;

    }else if (subscriber.type === 'evergreen-webhook') {
        return "Post to external server " + subscriber.target.url;
    }

    return ""
}
function addSubscriber($mdDialog, triggers, callback) {
    return subscriberPromise($mdDialog, "Add", triggers, callback)
}

function editSubscriber($mdDialog, triggers, callback, trigger, subscriber) {
    return subscriberPromise($mdDialog, "Edit", triggers, callback, trigger, subscriber)
}

function subscriberPromise($mdDialog, verb, triggers, callback, trigger, subscriber) {
    return $mdDialog.alert({
        title:"test",
        templateUrl: "static/partials/subscription_modal.html",
        controller: subCtrl,
        locals: {
            triggers: triggers,
            verb: verb,
            callback: callback,
            trigger: trigger,
            subscriber: subscriber
        },
    });
}

function subCtrl($scope, $mdDialog, verb, triggers, callback, trigger, subscriber) {
    // labels should complete the following sentence fragment:
    // 'then notify by ...'
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
            callback($scope.trigger, subscriber);
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
        if ($scope.trigger == null || $scope.method == null) {
            return false;
        }
        if ($scope.targets[$scope.method.value] == null) {
            return false
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
    if (subscriber !== undefined) {
        $scope.targets[subscriber.type] = subscriber.target;
        t = _.filter($scope.subscription_methods, function(t) { return t.value == subscriber.type; });
        if (t.length === 1) {
            $scope.method = t[0];
        }
        t = _.filter($scope.triggers, function(t) { return t.trigger == trigger; });
        if (t.length === 1) {
            $scope.trigger = t[0];
        }
    }
    $scope.subscriber = {};
}
