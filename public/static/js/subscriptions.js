const SUBSCRIPTION_JIRA_COMMENT = 'jira-comment';
const SUBSCRIPTION_JIRA_ISSUE = 'jira-issue';
const SUBSCRIPTION_SLACK = 'slack';
const SUBSCRIPTION_EMAIL = 'email';
const SUBSCRIPTION_EVERGREEN_WEBHOOK = 'evergreen-webhook';

function subscriberLabel(subscriber) {
    if (subscriber.type === SUBSCRIPTION_JIRA_COMMENT) {
        return "Post a comment on JIRA issue " + subscriber.target;

    }else if (subscriber.type === SUBSCRIPTION_JIRA_ISSUE) {
        return "Create a JIRA issue in " + subscriber.target;

    }else if (subscriber.type === SUBSCRIPTION_SLACK) {
        return "Send a slack message to " + subscriber.target;

    }else if (subscriber.type === SUBSCRIPTION_EMAIL) {
        return "Send an email to " + subscriber.target;

    }else if (subscriber.type === SUBSCRIPTION_EVERGREEN_WEBHOOK) {
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
// trigger and subscriber are the selected trigger and subscriber
function editSubscriber($mdDialog, triggers, subscription) {
    return subscriberPromise($mdDialog, "Edit", triggers, subscription)
}

function subscriberPromise($mdDialog, verb, triggers, subscription) {
    return $mdDialog.confirm({
        title:"test",
        templateUrl: "static/partials/subscription_modal.html",
        controllerAs: "c",
        controller: subCtrl,
        bindToController: true,
        locals: {
            triggers: triggers,
            verb: verb,
            subscription: subscription
        },
    });
}

function subCtrl($scope, $mdDialog) {
    // labels should complete the following sentence fragments:
    // 'then notify by ...'
    // 'when ...'
    $scope.subscription_methods = [
        {
            value: SUBSCRIPTION_EMAIL,
            label: "sending an email",
        },
        {
            value: SUBSCRIPTION_SLACK,
            label: "sending a slack message",
        },
        {
            value: SUBSCRIPTION_JIRA_COMMENT,
            label: "making a comment on a JIRA issue",
        },
        {
            value: SUBSCRIPTION_JIRA_ISSUE,
            label: "making a JIRA issue",
        },
        {
            value: SUBSCRIPTION_EVERGREEN_WEBHOOK,
            label: "posting to an external server",
        },
        // Github status api is deliberately omitted here
    ];

    $scope.closeDialog = function(save) {
        console.log(save)
        if(save === true) {
            subscriber = {
                type: $scope.method.value,
                target: $scope.targets[$scope.method.value],
            }
            subscriber.label = subscriberLabel(subscriber);

            d = {
                subscriber: subscriber,
                resource_type: $scope.trigger.resource_type,
                trigger: $scope.trigger.trigger,
                trigger_label: $scope.trigger.label
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

        if ($scope.method.value === SUBSCRIPTION_JIRA_COMMENT) {
            return $scope.targets[SUBSCRIPTION_JIRA_COMMENT].match(".+-[0-9]+") !== null

        }else if ($scope.method.value === SUBSCRIPTION_JIRA_ISSUE) {
            return $scope.targets[SUBSCRIPTION_JIRA_ISSUE].match(".+") !== null

        }else if ($scope.method.value === SUBSCRIPTION_SLACK) {
            return $scope.targets[SUBSCRIPTION_SLACK].match("(#|@).+") !== null

        }else if ($scope.method.value === SUBSCRIPTION_EMAIL) {
            return $scope.targets[SUBSCRIPTION_EMAIL].match(".+@.+") !== null

        }else if ($scope.method.value === SUBSCRIPTION_EVERGREEN_WEBHOOK) {
            if ($scope.targets[SUBSCRIPTION_EVERGREEN_WEBHOOK] == undefined) {
                return false;
            }
            return ($scope.targets[SUBSCRIPTION_EVERGREEN_WEBHOOK].secret.length >= 32 &&
                $scope.targets[SUBSCRIPTION_EVERGREEN_WEBHOOK].url.match("https://.+") !== null)
        }

        return false;
    };

    $scope.method = {};
    $scope.targets = {
        SUBSCRIPTION_EVERGREEN_WEBHOOK: {
            secret: $scope.generateSecret(),
        },
    };
    if ($scope.c.subscription !== undefined) {
        $scope.targets[$scope.c.subscription.subscriber.type] = $scope.c.subscription.subscriber.target;
        t = _.filter($scope.subscription_methods, function(t) { return t.value == $scope.c.subscription.subscriber.type; });
        if (t.length === 1) {
            $scope.method = t[0];
        }
        $scope.trigger = lookupTrigger($scope.c.triggers, $scope.c.subscription.trigger, $scope.c.subscription.resource_type);
    }
}
