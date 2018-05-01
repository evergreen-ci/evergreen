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

// triggers are javascript objects that look like this:
// {
//      trigger: "trigger-name",
//      label: "human readable name for trigger that completes the fragment 'when ...'"
//      resource_type: "event resource type, like PATCH, HOST, etc."
// }

// Return a promise for the add subscription modal, with the list of triggers
function addSubscriber($mdDialog, triggers) {
    return subscriberPromise($mdDialog, "Add", triggers)
}

// Return a promise for the edit subscription modal, with the list of triggers.
// "trigger"
function editSubscriber($mdDialog, triggers, trigger, subscriber) {
    return subscriberPromise($mdDialog, "Edit", triggers, trigger, subscriber)
}

function subscriberPromise($mdDialog, verb, triggers, trigger, subscriber) {
    return $mdDialog.confirm({
        title:"test",
        templateUrl: "static/partials/subscription_modal.html",
        controllerAs: "c",
        controller: subCtrl,
        bindToController: true,
        locals: {
            triggers: triggers,
            verb: verb,
            trigger: trigger,
            subscriber: subscriber
        },
    });
}

function subCtrl($scope, $mdDialog) {
    // labels should complete the following sentence fragments:
    // 'then notify by ...'
    // 'when ...'
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
    $scope.closeDialog = function(save) {
        if(save === true) {
            subscriber = {
                type: $scope.method.value,
                target: $scope.targets[$scope.method.value],
            }
            subscriber.label = subscriberLabel(subscriber);
            d = {
                subscriber: subscriber,
                trigger: $scope.trigger.trigger,
                trigger_data: $scope.trigger
            };
            $mdDialog.hide(d);
        }
        $mdDialog.cancel();
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
    if ($scope.c.subscriber !== undefined) {
        $scope.targets[$scope.c.subscriber.type] = $scope.c.subscriber.target;
        t = _.filter($scope.subscription_methods, function(t) { return t.value == $scope.c.subscriber.type; });
        if (t.length === 1) {
            $scope.method = t[0];
        }
        t = _.filter($scope.c.triggers, function(t) { return t.trigger == $scope.c.trigger; });
        if (t.length === 1) {
            $scope.trigger = t[0];
        }
    }
}
