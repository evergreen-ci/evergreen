const SUBSCRIPTION_JIRA_COMMENT = 'jira-comment';
const SUBSCRIPTION_JIRA_ISSUE = 'jira-issue';
const SUBSCRIPTION_SLACK = 'slack';
const SUBSCRIPTION_EMAIL = 'email';
const SUBSCRIPTION_EVERGREEN_WEBHOOK = 'evergreen-webhook';
const DEFAULT_SUBSCRIPTION_METHODS = [
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

// Return a human readable label for a given subscriber object
function subscriberLabel(subscriber) {
    if (subscriber.type === SUBSCRIPTION_JIRA_COMMENT) {
        return "Post a comment on JIRA issue " + subscriber.target;

    }else if (subscriber.type === SUBSCRIPTION_JIRA_ISSUE) {
        return "Create a JIRA issue in " + subscriber.target.project +
            " with issue type " + subscriber.target.issue_type;

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
function addSubscriber($mdDialog, triggers, omitMethods) {
    return subscriberPromise($mdDialog, "Add", triggers, omitMethods)
}

// Return a promise for the edit subscription modal, with the list of triggers.
// trigger and subscriber are the selected trigger and subscriber
function editSubscriber($mdDialog, triggers, subscription, omitMethods) {
    return subscriberPromise($mdDialog, "Edit", triggers, omitMethods, subscription)
}

function subscriberPromise($mdDialog, verb, triggers, omitMethods, subscription) {
    return $mdDialog.confirm({
        title:"test",
        templateUrl: "/static/partials/subscription_modal.html",
        controllerAs: "c",
        controller: subCtrl,
        bindToController: true,
        locals: {
            triggers: triggers,
            verb: verb,
            subscription: subscription,
            omit: omitMethods
        },
    });
}

function subCtrl($scope, $mdDialog, mciUserSettingsService) {
    // labels should complete the following sentence fragments:
    // 'then notify by ...'
    // 'when ...'

    $scope.subscription_methods = DEFAULT_SUBSCRIPTION_METHODS;
    if ($scope.c.omit) {
      $scope.subscription_methods = _($scope.subscription_methods).filter(function(method){
        return !$scope.c.omit[method.value];
      });
    };
    $scope.extraData = {};
    $scope.regexSelectors = {};
    $scope.tempRegexSelector = {};

    $scope.closeDialog = function(save) {
        if(save === true) {
            $scope.validationErrors = [];
            for (var key in $scope.customValidation) {
              var validationMsg = $scope.customValidation[key]($scope.extraData[key]);
              if (validationMsg) {
                $scope.validationErrors.push(validationMsg);
              };
            };
            if ($scope.validationErrors.length > 0) {
              return;
            }
            subscriber = {
                type: $scope.method.value,
                target: $scope.targets[$scope.method.value],
            }
            subscriber.label = subscriberLabel(subscriber);

            d = $scope.c.subscription || {};
            d.subscriber = subscriber;
            d.resource_type = $scope.trigger.resource_type;
            d.trigger = $scope.trigger.trigger;
            d.trigger_label = $scope.trigger.label;
            d.trigger_data = $scope.extraData;
            d.regex_selectors = _($scope.regexSelectors).map(function(val, key) {
              return {type: key, data: val.data};
            });
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

    $scope.addCustomValidation = function(fields) {
      $scope.customValidation = {};
      if (fields) {
        _.each(fields, function(field) {
          $scope.customValidation[field.key] = field.validator;
        });
      };
    }

    $scope.valid = function() {
        if (!$scope.trigger || !$scope.method) {
            return false;
        }
        if (!$scope.targets[$scope.method.value]) {
            return false
        }

        if ($scope.method.value === SUBSCRIPTION_JIRA_COMMENT) {
            return $scope.targets[SUBSCRIPTION_JIRA_COMMENT].match(".+-[0-9]+") !== null

        }else if ($scope.method.value === SUBSCRIPTION_JIRA_ISSUE) {
            if (!$scope.targets[SUBSCRIPTION_JIRA_ISSUE]) {
                return false;
            }

            return $scope.targets[SUBSCRIPTION_JIRA_ISSUE]['project'].match(".+") !== null &&
                $scope.targets[SUBSCRIPTION_JIRA_ISSUE]['issue_type'].match(".+") !== null;

        }else if ($scope.method.value === SUBSCRIPTION_SLACK) {
            return $scope.targets[SUBSCRIPTION_SLACK].match("(#|@).+") !== null

        }else if ($scope.method.value === SUBSCRIPTION_EMAIL) {
            return $scope.targets[SUBSCRIPTION_EMAIL].match(".+@.+") !== null

        }else if ($scope.method.value === SUBSCRIPTION_EVERGREEN_WEBHOOK) {
            if (!$scope.targets[SUBSCRIPTION_EVERGREEN_WEBHOOK]) {
                return false;
            }
            if (!$scope.targets[SUBSCRIPTION_EVERGREEN_WEBHOOK].url ||
                !$scope.targets[SUBSCRIPTION_EVERGREEN_WEBHOOK].secret) {
                return false;
            }

            return ($scope.targets[SUBSCRIPTION_EVERGREEN_WEBHOOK].secret.length >= 32 &&
                $scope.targets[SUBSCRIPTION_EVERGREEN_WEBHOOK].url.match("https://.+") !== null)
        }

        return false;
    };

    $scope.bindTrigger = function() {
      _.each($scope.c.triggers, function(trigger){
        if (trigger.resource_type === $scope.trigger.resource_type &&
              trigger.trigger === $scope.trigger.trigger) {
          $scope.extraFields = trigger.extraFields;
          $scope.regexSelectorOptions = trigger.regex_selectors;
          $scope.addCustomValidation(trigger.extraFields);
          return;
        }
      });
      $scope.regexSelectors = {};
    };
    $scope.method = {};
    $scope.targets = {};
    $scope.targets[SUBSCRIPTION_EVERGREEN_WEBHOOK] = {
            secret: $scope.generateSecret(),
    };
    if ($scope.c.subscription) {
        $scope.targets[$scope.c.subscription.subscriber.type] = $scope.c.subscription.subscriber.target;
        t = _.filter($scope.subscription_methods, function(t) { return t.value == $scope.c.subscription.subscriber.type; });
        if (t.length === 1) {
            $scope.method = t[0];
        }
        $scope.trigger = lookupTrigger($scope.c.triggers, $scope.c.subscription.trigger, $scope.c.subscription.resource_type);

    }else {
        mciUserSettingsService.getUserSettings({success: function(resp) {
            if (!$scope.targets[SUBSCRIPTION_SLACK]) {
                $scope.targets[SUBSCRIPTION_SLACK] = "@" + resp.data.slack_username || "";
            }
        }, error: function(resp) {
            console.log("failed to fetch user settings: ", resp);
        }});
    }

    $scope.addRegexSelector = function() {
      if (!$scope.tempRegexSelector.type || !$scope.tempRegexSelector.data) {
          return;
      }
      var typeLabel = _.where($scope.trigger.regex_selectors, {type: $scope.tempRegexSelector.type })[0];
      $scope.regexSelectors[$scope.tempRegexSelector.type] = {type_label: typeLabel.type_label, data: $scope.tempRegexSelector.data};
      $scope.tempRegexSelector = {};
    }

    $scope.deleteRegexSelector = function(type) {
      delete $scope.regexSelectors[type];
    }

    $scope.selectorDisabled = function(field) {
      return field.type in $scope.regexSelectors;
    }

    $scope.hasSelectors = function() {
      return Object.keys($scope.regexSelectors).length !== 0;
    }
}

// Lookup a trigger with given (name, resource_type) pair in triggers, an
// array of of trigger objects, as described above.
// returns trigger object, or null
function lookupTrigger(triggers, trigger, resource_type) {
    t = _.filter(triggers, function(t) {
        return t.trigger == trigger && t.resource_type == resource_type;
    });
    if (t.length === 1) {
        return t[0];
    }

    return null;
}

function addSelectorsAndOwnerType(subscription, type, id) {
  if (!subscription) {
    return;
  }
  if (!subscription.selectors) {
    subscription.selectors = [];
  }
  subscription.selectors.push({
    type: "object",
    data: type
  });
  subscription.selectors.push({
    type: "id",
    data: id
  });
  subscription.owner_type = "person";
};

function addInSelectorsAndOwnerType(subscription, type, inType, id) {
  if (!subscription) {
    return;
  }
  if (!subscription.selectors) {
    subscription.selectors = [];
  }
  subscription.selectors.push({
    type: "object",
    data: type
  });
  subscription.selectors.push({
    type: "in-" + inType,
    data: id
  });
  subscription.owner_type = "person";
}

function validateDuration(duration) {
  if (!Number.isInteger(+duration)) {
    return duration + " must be an integer";
  }
  if (+duration < 0) {
    return duration + " cannot be negative";
  }
  return "";
}
function validatePercentage(percent) {
  if (!isFinite(percent)) {
    return percent + " must be a number";
  }
  if (+percent <= 0) {
    return percent + " must be positive";
  }
  return "";
}

function buildRegexSelectors() {
  return [
    {
      type: "display-name",
      type_label: "Build Variant Name",
    },
    {
      type: "build-variant",
      type_label: "Build Variant ID",
    },
  ];
}

function taskRegexSelectors() {
  return [
    {
      type: "display-name",
      type_label: "Task Name",
    }
  ];
}
