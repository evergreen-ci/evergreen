mciModule.controller(
  "ProjectCtrl",
  function ($scope, $window, $http, $location, $mdDialog, timeUtil) {
    $scope.availableTriggers = $window.availableTriggers;
    $scope.userId = $window.user.Id;
    $scope.create = $window.canCreate;
    $scope.validDefaultLoggers = $window.validDefaultLoggers;

    $scope.projectVars = {};
    $scope.patchVariants = [];
    $scope.projectRef = {};
    $scope.displayName = "";

    $scope.projectView = false;

    $scope.settingsFormData = {};
    $scope.saveMessage = "";

    $scope.modalTitle = "New Project";
    $scope.modalOpen = false;
    $scope.newProject = {};
    $scope.newProjectMessage = "";

    $scope.repoChanged = false;
    $scope.repoChange = function () {
      if ($scope.repoChanged == false) {
        $scope.repoChanged = true;
      }
    };

    $scope.isDirty = false;
    const failureTypeSubscriberConfig = {
      text: "Failure type",
      key: "failure-type",
      type: "select",
      options: {
        any: "Any",
        test: "Test",
        system: "System",
        setup: "Setup",
      },
      default: "any",
    };
    const requesterSubscriberConfig = {
      text: "Build initiator",
      key: "requester",
      type: "select",
      options: {
        gitter_request: "Commit",
        patch_request: "Patch",
        github_pull_request: "Pull Request",
        merge_test: "Commit Queue",
        ad_hoc: "Periodic Build",
      },
      default: "gitter_request",
    };
    $scope.triggers = [{
        trigger: "outcome",
        resource_type: "VERSION",
        label: "any version finishes",
        extraFields: [requesterSubscriberConfig],
      },
      {
        trigger: "failure",
        resource_type: "VERSION",
        label: "any version fails",
        extraFields: [requesterSubscriberConfig],
      },
      {
        trigger: "outcome",
        resource_type: "BUILD",
        label: "any build finishes",
        regex_selectors: buildRegexSelectors(),
        extraFields: [requesterSubscriberConfig],
      },
      {
        trigger: "failure",
        resource_type: "BUILD",
        label: "any build fails",
        regex_selectors: buildRegexSelectors(),
        extraFields: [requesterSubscriberConfig],
      },
      {
        trigger: "outcome",
        resource_type: "TASK",
        label: "any task finishes",
        regex_selectors: taskRegexSelectors(),
        extraFields: [requesterSubscriberConfig],
      },
      {
        trigger: "failure",
        resource_type: "TASK",
        label: "any task fails",
        regex_selectors: taskRegexSelectors(),
        extraFields: [failureTypeSubscriberConfig, requesterSubscriberConfig],
      },
      {
        trigger: "first-failure-in-version",
        resource_type: "TASK",
        label: "the first failure in a version occurs",
        regex_selectors: taskRegexSelectors(),
        extraFields: [requesterSubscriberConfig],
      },
      {
        trigger: "first-failure-in-build",
        resource_type: "TASK",
        label: "the first failure in each build occurs",
        regex_selectors: taskRegexSelectors(),
        extraFields: [requesterSubscriberConfig],
      },
      {
        trigger: "first-failure-in-version-with-name",
        resource_type: "TASK",
        label: "the first failure in each version for each task name occurs",
        regex_selectors: taskRegexSelectors(),
        extraFields: [requesterSubscriberConfig],
      },
      {
        trigger: "regression",
        resource_type: "TASK",
        label: "a previously passing task fails",
        regex_selectors: taskRegexSelectors(),
        extraFields: [{
            text: "Re-notify after how many hours",
            key: "renotify-interval",
            validator: validateDuration,
            default: "48",
          },
          failureTypeSubscriberConfig,
        ],
      },
      {
        trigger: "regression-by-test",
        resource_type: "TASK",
        label: "a previously passing test in a task fails",
        regex_selectors: taskRegexSelectors(),
        extraFields: [{
            text: "Test names matching regex",
            key: "test-regex",
            validator: null,
          },
          {
            text: "Re-notify after how many hours",
            key: "renotify-interval",
            validator: validateDuration,
            default: "48",
          },
          failureTypeSubscriberConfig,
        ],
      },
      {
        trigger: "exceeds-duration",
        resource_type: "TASK",
        label: "the runtime for a task exceeds some duration",
        regex_selectors: taskRegexSelectors(),
        extraFields: [{
          text: "Task duration (seconds)",
          key: "task-duration-secs",
          validator: validateDuration,
        }, ],
      },
      {
        trigger: "runtime-change",
        resource_type: "TASK",
        label: "the runtime for a successful task changes by some percentage",
        regex_selectors: taskRegexSelectors(),
        extraFields: [{
          text: "Percent change",
          key: "task-percent-change",
          validator: validatePercentage,
        }, ],
      },
    ];

    // refreshTrackedProjects will populate the list of projects that should be displayed
    // depending on the user.
    $scope.refreshTrackedProjects = function (trackedProjects) {
      $scope.trackedProjects = trackedProjects;
      $scope.enabledProjects = _.filter(
        $scope.trackedProjects,
        _.property("enabled")
      );
      $scope.disabledProjects = _.filter(
        $scope.trackedProjects,
        _.negate(_.property("enabled"))
      );
    };

    $scope.refreshTrackedProjects($window.allTrackedProjects);

    $scope.showProject = function (project) {
      return !(project.length == 0);
    };

    $scope.isBatchTimeValid = function (t) {
      if (t == "") {
        return true;
      }
      return !isNaN(Number(t)) && Number(t) >= 0;
    };

    $scope.shouldDisableWebhook = function () {
      return $scope.settingsFormData.build_baron_settings
          && (($scope.settingsFormData.build_baron_settings.ticket_search_projects !== null
              && $scope.settingsFormData.build_baron_settings.ticket_search_projects !== undefined
              && $scope.settingsFormData.build_baron_settings.ticket_search_projects.length > 0)
          || $scope.settingsFormData.build_baron_settings.ticket_create_project
          || $scope.ticket_search_project);
    };

    $scope.shouldDisableBB = function () {
      return $scope.settingsFormData.task_annotation_settings && ($scope.settingsFormData.task_annotation_settings.web_hook.endpoint
          || $scope.settingsFormData.task_annotation_settings.web_hook.secret);
    };

    $scope.bbConfigIsValid = function () {
      if ($scope.settingsFormData.build_baron_settings.ticket_create_project){
          return $scope.settingsFormData.build_baron_settings.ticket_search_projects !== null
              && $scope.settingsFormData.build_baron_settings.ticket_search_projects !== undefined
              && $scope.settingsFormData.build_baron_settings.ticket_search_projects.length > 0
      }
      else {
          return $scope.settingsFormData.build_baron_settings.ticket_search_projects === null
              || $scope.settingsFormData.build_baron_settings.ticket_search_projects === undefined
              || $scope.settingsFormData.build_baron_settings.ticket_search_projects.length <= 0
      }
    };

    $scope.findProject = function (identifier) {
      return _.find($scope.trackedProjects, function (project) {
        return project.identifier == identifier;
      });
    };

    $scope.openAlertModal = function () {
      var modal = $("#alert-modal").modal("show");
    };

    $scope.openAdminModal = function (opt) {
      var modal = $("#admin-modal").modal("show");
      $scope.newProjectMessage = "";
      if (opt === "newProject") {
        $("#project-name").focus();
        modal.on("shown.bs.modal", function () {
          $scope.modalOpen = true;
        });

        modal.on("hide.bs.modal", function () {
          $scope.modalOpen = false;
        });
      }
    };

    // addJiraField adds a jira field to the task annotation settings
    $scope.addJiraField = function () {
      if(!$scope.settingsFormData.task_annotation_settings.jira_custom_fields){
          $scope.settingsFormData.task_annotation_settings.jira_custom_fields = []
      }
      $scope.settingsFormData.task_annotation_settings.jira_custom_fields.push({"field" : $scope.jira_field, "display_text" : $scope.jira_display_text});
      $scope.jira_display_text = "";
      $scope.jira_field = "";
    };

    // removeJiraField removes a jira field to the task annotation settings located at index
    $scope.removeJiraField = function (index) {
      $scope.settingsFormData.task_annotation_settings.jira_custom_fields.splice(index, 1);
      $scope.isDirty = true;
    };

    // addTicketSearchProject adds an ticket search project name to the build baron settings
    $scope.addTicketSearchProject = function () {
      if(!$scope.settingsFormData.build_baron_settings.ticket_search_projects){
          $scope.settingsFormData.build_baron_settings.ticket_search_projects = []
      }
      $scope.settingsFormData.build_baron_settings.ticket_search_projects.push($scope.ticket_search_project);
      $scope.ticket_search_project = "";
    };

    // removeTicketSearchProject removes the ticket search project name from the build baron settings located at index
    $scope.removeTicketSearchProject = function (index) {
      $scope.settingsFormData.build_baron_settings.ticket_search_projects.splice(index, 1);
      $scope.isDirty = true;
    };

    // addAdmin adds an admin name to the settingsFormData's list of admins
    $scope.addAdmin = function () {
      $scope.settingsFormData.admins.push($scope.admin_name);
      $scope.admin_name = "";
    };

    // removeAdmin removes the username located at index
    $scope.removeAdmin = function (index) {
      $scope.settingsFormData.admins.splice(index, 1);
      $scope.isDirty = true;
    };

    // addGitTagUser adds an admin name to the list of users authorized to create versions from git tags
    $scope.addGitTagUser = function () {
      $scope.settingsFormData.git_tag_authorized_users.push(
        $scope.git_tag_user_name
      );
      $scope.git_tag_user_name = "";
    };

    // removeGitTagUser removes the username located at index
    $scope.removeGitTagUser = function (index) {
      $scope.settingsFormData.git_tag_authorized_users.splice(index, 1);
      $scope.isDirty = true;
    };

    $scope.isValidGitTagUser = function(user) {
        if ($scope.settingsFormData.git_tag_authorized_users === undefined) {
            return true;
        }
        return !$scope.settingsFormData.git_tag_authorized_users.includes(user);
    }
    $scope.isValidGitTagTeam = function(team) {
        if ($scope.settingsFormData.git_tag_authorized_teams === undefined) {
            return true;
        }
        return !$scope.settingsFormData.git_tag_authorized_teams.includes(team);
    }
    $scope.isValidAdmin = function(admin) {
        if ($scope.settingsFormData.admins == undefined) {
            return true;
        }
        return !$scope.settingsFormData.admins.includes(admin);
    }

    $scope.addGitTagTeam = function () {
      $scope.settingsFormData.git_tag_authorized_teams.push(
        $scope.git_tag_team
      );
      $scope.git_tag_team = "";
    };

    $scope.removeGitTagTeam = function (index) {
      $scope.settingsFormData.git_tag_authorized_teams.splice(index, 1);
      $scope.isDirty = true;
    };

    // addCacheIgnoreFile adds a file pattern to the settingsFormData's list of ignored cache files
    $scope.addCacheIgnoreFile = function () {
      if (!$scope.settingsFormData.files_ignored_from_cache) {
        $scope.settingsFormData.files_ignored_from_cache = [];
      }
      $scope.settingsFormData.files_ignored_from_cache.push(
        $scope.cache_ignore_file_pattern
      );
      $scope.cache_ignore_file_pattern = "";
    };

    $scope.addGithubTrigger = function () {
      $scope.settingsFormData.github_trigger_aliases.push(
        $scope.added_trigger_aliases
      );
      $scope.added_trigger_aliases = "";
    };

    $scope.removeGithubTrigger = function (index) {
      $scope.settingsFormData.github_trigger_aliases.splice(index, 1);
      $scope.isDirty = true;
    };

    // removeCacheIgnoreFile removes the file pattern located at index
    $scope.removeCacheIgnoreFile = function (index) {
      $scope.settingsFormData.files_ignored_from_cache.splice(index, 1);
      $scope.isDirty = true;
    };

    $scope.addProject = function () {
      $scope.modalOpen = false;
      $("#admin-modal").modal("hide");

      if ($scope.findProject($scope.newProject.identifier)) {
        console.log("project identifier already exists");
        $scope.newProjectMessage = "Project name already exists.";
        $scope.newProject = {};
        return;
      }

      // if copyProject is set, copy the current project
      if ($scope.newProject.copyProject) {
        $scope.settingsFormData.batch_time = parseInt(
          $scope.settingsFormData.batch_time
        );
        $http
          .put("/project/" + $scope.newProject.identifier, $scope.newProject)
          .then(
            function (resp) {
              var data_put = resp.data;
              item = Object.assign({}, $scope.settingsFormData);
              item.identifier = $scope.newProject.identifier;
              item.pr_testing_enabled = false;
              item.commit_queue.enabled = false;
              item.commit_queue.require_signed = false;
              item.git_tag_versions_enabled = false;
              item.github_checks_enabled = false;
              item.enabled = false;
              item.subscriptions = _.filter($scope.subscriptions, function (d) {
                return !d.changed;
              });
              // project variables will be copied in the route, so that we get private variables correctly
              item.project_vars = null;
              item.private_vars = null;
              item.restricted_vars = null;
              $http.post("/project/" + $scope.newProject.identifier, item).then(
                function (resp) {
                  $scope.refreshTrackedProjects(data_put.AllProjects);
                  $scope.loadProject(data_put.ProjectId);
                  $scope.newProject = {};
                },
                function (resp) {
                  console.log(
                    "error saving data for new project: " + resp.status
                  );
                }
              );
            },
            function (resp) {
              console.log("error creating new project: " + resp.status);
            }
          );

        // otherwise, create a blank project
      } else {
        $http
          .put("/project/" + $scope.newProject.identifier, $scope.newProject)
          .then(
            function (resp) {
              var data = resp.data;
              $scope.refreshTrackedProjects(data.AllProjects);
              $scope.loadProject(data.ProjectId);
              $scope.newProject = {};
              $scope.settingsFormData.tracks_push_events = true;
            },
            function (resp) {
              console.log("error creating project: " + resp.status);
            }
          );
      }
    };

    $scope.loadProject = function (projectId) {
      $http.get("/project/" + projectId).then(
        function (resp) {
          var data = resp.data;
          $scope.projectView = true;
          $scope.projectRef = data.ProjectRef;
          if (data.ProjectVars === null) {
            data.ProjectVars = {};
          }
          if (data.ProjectRef === null) {
            data.ProjectRef = {};
          }
          $scope.projectVars = data.ProjectVars.vars || {};
          $scope.privateVars = data.ProjectVars.private_vars || {};
          $scope.restrictedVars = data.ProjectVars.restricted_vars || {};
          $scope.github_webhooks_enabled = data.github_webhooks_enabled;
          $scope.githubChecksConflicts =
            data.github_checks_conflicting_refs || [];
          $scope.prTestingConflicts = data.pr_testing_conflicting_refs || [];
          $scope.prTestingEnabled = data.ProjectRef.pr_testing_enabled || false;
          $scope.commitQueueConflicts =
            data.commit_queue_conflicting_refs || [];
          $scope.project_triggers = data.ProjectRef.triggers || [];
          $scope.patch_trigger_aliases =
            data.ProjectRef.patch_trigger_aliases || [];
          $scope.periodic_builds = data.ProjectRef.periodic_builds || [];
          $scope.permissions = data.permissions || {};
          $scope.github_valid_orgs = data.github_valid_orgs;
          $scope.cur_command = {};
          _.each($scope.project_triggers, function (trigger) {
            if (trigger.command) {
              trigger.file = trigger.generate_file;
            } else {
              trigger.file = trigger.config_file;
            }
          });

          $scope.aliases = _.sortBy(data.aliases || [], function (v) {
            return v.alias + v.variant + v.task;
          });

          $scope.settingsFormData = {
            id: $scope.projectRef.id,
            identifier: $scope.projectRef.identifier,
            project_vars: $scope.projectVars,
            private_vars: $scope.privateVars,
            restricted_vars: $scope.restrictedVars,
            display_name: $scope.projectRef.display_name,
            default_logger: $scope.projectRef.default_logger,
            cedar_test_results_enabled: $scope.projectRef.cedar_test_results_enabled,
            remote_path: $scope.projectRef.remote_path,
            spawn_host_script_path: $scope.projectRef.spawn_host_script_path,
            batch_time: parseInt($scope.projectRef.batch_time),
            deactivate_previous: $scope.projectRef.deactivate_previous,
            relative_url: $scope.projectRef.relative_url,
            branch_name: $scope.projectRef.branch_name || "main",
            owner_name: $scope.projectRef.owner_name,
            repo_name: $scope.projectRef.repo_name,
            enabled: $scope.projectRef.enabled,
            private: $scope.projectRef.private,
            restricted: $scope.projectRef.restricted,
            patching_disabled: $scope.projectRef.patching_disabled,
            dispatching_disabled: $scope.projectRef.dispatching_disabled,
            task_sync: $scope.projectRef.task_sync,
            repotracker_disabled: $scope.projectRef.repotracker_disabled,
            alert_config: $scope.projectRef.alert_config || {},
            repotracker_error: $scope.projectRef.repotracker_error || {},
            admins: $scope.projectRef.admins || [],
            git_tag_authorized_users: $scope.projectRef.git_tag_authorized_users || [],
            git_tag_authorized_teams: $scope.projectRef.git_tag_authorized_teams || [],
            tracks_push_events: data.ProjectRef.tracks_push_events || false,
            pr_testing_enabled: data.ProjectRef.pr_testing_enabled || false,
            git_tag_versions_enabled: data.ProjectRef.git_tag_versions_enabled || false,
            github_checks_enabled: data.ProjectRef.github_checks_enabled || false,
            commit_queue: data.ProjectRef.commit_queue || {},
            workstation_config: data.ProjectRef.workstation_config || {},
            notify_on_failure: $scope.projectRef.notify_on_failure,
            force_repotracker_run: false,
            delete_aliases: [],
            delete_subscriptions: [],
            files_ignored_from_cache: data.ProjectRef.files_ignored_from_cache,
            github_trigger_aliases: data.ProjectRef.github_trigger_aliases || [],
            disabled_stats_cache: data.ProjectRef.disabled_stats_cache,
            periodic_builds: data.ProjectRef.periodic_builds,
            use_repo_settings: $scope.projectRef.use_repo_settings,
            build_baron_settings: data.ProjectRef.build_baron_settings || {},
            task_annotation_settings: data.ProjectRef.task_annotation_settings || {},
            perf_enabled: data.ProjectRef.perf_enabled || false,
          };
          // Divide aliases into categories
          $scope.settingsFormData.github_aliases = $scope.aliases.filter(
            function (d) {
              return d.alias === "__github";
            }
          );
          $scope.settingsFormData.github_checks_aliases = $scope.aliases.filter(
            function (d) {
              return d.alias === "__github_checks";
            }
          );
          $scope.settingsFormData.commit_queue_aliases = $scope.aliases.filter(
            function (d) {
              return d.alias === "__commit_queue";
            }
          );
          $scope.settingsFormData.git_tag_aliases = $scope.aliases.filter(
            function (d) {
              return d.alias === "__git_tag";
            }
          );
          $scope.settingsFormData.patch_aliases = $scope.aliases.filter(
            function (d) {
              return (
                d.alias !== "__github" &&
                d.alias !== "__github_checks" &&
                d.alias !== "__commit_queue" &&
                d.alias !== "__git_tag"
              );
            }
          );

          // Set commit queue defaults
          if (!$scope.settingsFormData.commit_queue.merge_method) {
            $scope.settingsFormData.commit_queue.merge_method =
              $scope.validMergeMethods[0];
          }
          $scope.subscriptions = _.map(data.subscriptions || [], function (v) {
            t = lookupTrigger($scope.triggers, v.trigger, v.resource_type);
            if (!t) {
              return v;
            }
            v.trigger_label = t.label;
            v.subscriber.label = subscriberLabel(v.subscriber);
            return v;
          });

          $scope.displayName = $scope.projectRef.display_name ?
            $scope.projectRef.display_name :
            $scope.projectRef.identifier;
          $location.hash($scope.projectRef.identifier);
          $scope.$emit(
            "loadProject",
            $scope.projectRef.identifier,
            $scope.displayName
          );
        },
        function (resp) {
          console.log(resp.status);
        }
      );
    };

    $scope.$on("$locationChangeStart", function (event) {
      $scope.hashLoad();
    });

    $scope.hashLoad = function () {
      var projectHash = $location.hash();
      if (projectHash) {
        // If the project in the hash exists and is not the current project, load it.
        if (
          _.contains(
            _.pluck($scope.trackedProjects, "identifier"),
            projectHash
          ) &&
          $scope.projectRef.identifier != projectHash
        ) {
          $scope.loadProject(projectHash);
        }
      }
    };

    $scope.valueString = function (name, value) {
      if ($scope.privateVars[name]) {
        return "{REDACTED}";
      }
      return value;
    };

    $scope.shouldHighlight = function (project) {
      if ($scope.projectRef) {
        return project.identifier == $scope.projectRef.identifier;
      }
      return false;
    };

    $scope.saveProject = function () {
      $scope.settingsFormData.batch_time = parseInt(
        $scope.settingsFormData.batch_time
      );
      $scope.settingsFormData.triggers = $scope.project_triggers;
      $scope.settingsFormData.patch_trigger_aliases =
        $scope.patch_trigger_aliases;
      $scope.settingsFormData.periodic_builds = $scope.periodic_builds;
      _.each($scope.settingsFormData.triggers, function (trigger) {
        if (trigger.command) {
          trigger.generate_file = trigger.file;
          trigger.config_file = "";
        } else {
          trigger.config_file = trigger.file;
          trigger.generate_file = "";
        }
        if (trigger.level === "build") {
          delete trigger.task_regex;
        }
      });
      if ($scope.proj_var) {
        $scope.addProjectVar();
      }
      if ($scope.github_alias) {
        $scope.addGithubAlias();
      }

      if ($scope.github_checks_alias) {
        $scope.addGithubChecksAlias();
      }

      if ($scope.commit_queue_alias) {
        $scope.addCommitQueueAlias();
      }

      if ($scope.patch_alias) {
        $scope.addPatchAlias();
      }

      $scope.settingsFormData.subscriptions = _.filter(
        $scope.subscriptions,
        function (d) {
          return d.changed;
        }
      );

      if ($scope.admin_name) {
        $scope.addAdmin();
      }

      if ($scope.ticket_search_project) {
        $scope.addTicketSearchProject();
      }

      if ($scope.git_tag_user_name) {
        $scope.addGitTagUser();
      }

      if ($scope.git_tag_team) {
        $scope.addGitTagTeam();
      }

      $http
        .post("/project/" + $scope.settingsFormData.id, $scope.settingsFormData)
        .then(
          function (resp) {
            var data = resp.data;
            $scope.saveMessage = "Settings Saved.";
            $scope.refreshTrackedProjects(data.AllProjects);
            $scope.settingsForm.$setPristine();
            $scope.settingsFormData.force_repotracker_run = false;
            $scope.loadProject($scope.settingsFormData.id);
            $scope.isDirty = false;
          },
          function (resp) {
            const message = (resp.data.error ? resp.data.error : resp.data);
            $scope.saveMessage = "Couldn't save project: " + message;
          }
        );
    };

    $scope.addProjectVar = function () {
      if ($scope.proj_var.name && $scope.proj_var.value) {
        $scope.settingsFormData.project_vars[$scope.proj_var.name] =
          $scope.proj_var.value;
        if ($scope.proj_var.is_private) {
          $scope.settingsFormData.private_vars[$scope.proj_var.name] = true;
        }
        if ($scope.proj_var.is_restricted) {
          $scope.settingsFormData.restricted_vars[$scope.proj_var.name] = true;
        }

        $scope.proj_var.name = "";
        $scope.proj_var.value = "";
        $scope.proj_var.is_private = false;
        $scope.proj_var.is_restricted = false;
      }
    };

    $scope.addGithubAlias = function () {
      if (!$scope.validPatchDefinition($scope.github_alias)) {
        $scope.invalidGithubPatchDefinitionMessage =
          "A patch alias must have variant regex, and exactly one of task regex or tag";
        return;
      }
      var item = Object.assign({}, $scope.github_alias);
      item.alias = "__github";
      $scope.settingsFormData.github_aliases = $scope.settingsFormData.github_aliases.concat(
        [item]
      );
      delete $scope.github_alias;
      $scope.invalidGitHubPatchDefinitionMessage = "";
    };

    $scope.validGithubTrigger = function (trigger_alias) {
      return $scope.patch_trigger_aliases && $scope.patch_trigger_aliases
        .map((patch_trigger_alias) => patch_trigger_alias.alias)
        .includes(trigger_alias);
    };

    $scope.addGithubChecksAlias = function () {
      if (!$scope.validPatchDefinition($scope.github_checks_alias)) {
        $scope.invalidGithubPatchDefinitionMessage =
          "A patch alias must have variant regex, and exactly one of task regex or tag";
        return;
      }
      var item = Object.assign({}, $scope.github_checks_alias);
      item.alias = "__github_checks";
      $scope.settingsFormData.github_checks_aliases = $scope.settingsFormData.github_checks_aliases.concat(
        [item]
      );
      delete $scope.github_checks_alias;
      $scope.invalidGitHubPatchDefinitionMessage = "";
    };

    $scope.addCommitQueueAlias = function () {
      if (!$scope.validPatchDefinition($scope.commit_queue_alias)) {
        $scope.invalidCommitQueuePatchDefinitionMessage =
          "A patch alias must have variant regex, and exactly one of task regex or tag";
        return;
      }
      var item = Object.assign({}, $scope.commit_queue_alias);
      item.alias = "__commit_queue";
      $scope.settingsFormData.commit_queue_aliases = $scope.settingsFormData.commit_queue_aliases.concat(
        [item]
      );
      delete $scope.commit_queue_alias;
      $scope.invalidCommitQueuePatchDefinitionMessage = "";
    };

    $scope.addGitTagAlias = function () {
      if (!$scope.validGitTagVersionDefinition($scope.git_tag_alias)) {
        $scope.invalidGitTagAliasMessage =
          "An alias must have a git tag alias, variant regex, and exactly one of task regex or tag";
        return;
      }
      var item = Object.assign({}, $scope.git_tag_alias);
      item.alias = "__git_tag";
      $scope.settingsFormData.git_tag_aliases = $scope.settingsFormData.git_tag_aliases.concat(
        [item]
      );
      delete $scope.git_tag_alias;
      $scope.invalidGitTagAliasMessage = "";
    };

    $scope.addPatchAlias = function () {
      if (!$scope.validPatchAlias($scope.patch_alias)) {
        $scope.invalidPatchAliasMessage =
          "A patch alias must have an alias name, exactly one of variant regex or tag, and exactly one of task regex or tag";
        return;
      }
      var item = Object.assign({}, $scope.patch_alias);
      $scope.settingsFormData.patch_aliases = $scope.settingsFormData.patch_aliases.concat(
        [item]
      );
      delete $scope.patch_alias;
      $scope.invalidPatchAliasMessage = "";
    };

    $scope.addWorkstationCommand = function () {
      if (!$scope.settingsFormData.workstation_config) {
        $scope.settingsFormData.workstation_config = {};
      }
      if (!$scope.settingsFormData.workstation_config.setup_commands) {
        $scope.settingsFormData.workstation_config.setup_commands = [];
      }
      $scope.settingsFormData.workstation_config.setup_commands = $scope.settingsFormData.workstation_config.setup_commands.concat(
        $scope.cur_command
      );
      $scope.cur_command = {};
    };

    $scope.removeProjectVar = function (name) {
      delete $scope.settingsFormData.project_vars[name];
      delete $scope.settingsFormData.private_vars[name];
      delete $scope.settingsFormData.restricted_vars[name];
      $scope.isDirty = true;
    };

    $scope.removeGithubAlias = function (i) {
      if ($scope.settingsFormData.github_aliases[i]["_id"]) {
        $scope.settingsFormData.delete_aliases = $scope.settingsFormData.delete_aliases.concat(
          [$scope.settingsFormData.github_aliases[i]["_id"]]
        );
      }
      $scope.settingsFormData.github_aliases.splice(i, 1);
      $scope.isDirty = true;
    };

    $scope.removeGithubChecksAlias = function (i) {
      if ($scope.settingsFormData.github_checks_aliases[i]["_id"]) {
        $scope.settingsFormData.delete_aliases = $scope.settingsFormData.delete_aliases.concat(
          [$scope.settingsFormData.github_checks_aliases[i]["_id"]]
        );
      }
      $scope.settingsFormData.github_checks_aliases.splice(i, 1);
      $scope.isDirty = true;
    };

    $scope.removeCommitQueueAlias = function (i) {
      if ($scope.settingsFormData.commit_queue_aliases[i]["_id"]) {
        $scope.settingsFormData.delete_aliases = $scope.settingsFormData.delete_aliases.concat(
          [$scope.settingsFormData.commit_queue_aliases[i]["_id"]]
        );
      }
      $scope.settingsFormData.commit_queue_aliases.splice(i, 1);
      $scope.isDirty = true;
    };

    $scope.removeGitTagAlias = function (i) {
      if ($scope.settingsFormData.git_tag_aliases[i]["_id"]) {
        $scope.settingsFormData.delete_aliases = $scope.settingsFormData.delete_aliases.concat(
          [$scope.settingsFormData.git_tag_aliases[i]["_id"]]
        );
      }
      $scope.settingsFormData.git_tag_aliases.splice(i, 1);
      $scope.isDirty = true;
    };

    $scope.removePatchAlias = function (i) {
      if ($scope.settingsFormData.patch_aliases[i]["_id"]) {
        $scope.settingsFormData.delete_aliases = $scope.settingsFormData.delete_aliases.concat(
          [$scope.settingsFormData.patch_aliases[i]["_id"]]
        );
      }
      $scope.settingsFormData.patch_aliases.splice(i, 1);
      $scope.isDirty = true;
    };

    $scope.removeProjectTrigger = function (i) {
      if ($scope.project_triggers[i]) {
        $scope.project_triggers.splice(i, 1);
        $scope.isDirty = true;
      }
    };

    $scope.removePeriodicBuild = function (i) {
      if ($scope.periodic_builds[i]) {
        $scope.periodic_builds.splice(i, 1);
        $scope.isDirty = true;
      }
    };

    $scope.removePeriodicBuild = function (i) {
      if ($scope.settingsFormData.periodic_builds[i]) {
        $scope.settingsFormData.periodic_builds.splice(i, 1);
        $scope.isDirty = true;
      }
    };

    $scope.addPeriodicBuild = function () {
      $scope.invalidPeriodicBuildMsg = $scope.periodicBuildErrors(
        $scope.periodic_build
      );
      if ($scope.invalidPeriodicBuildMsg !== "") {
        return;
      }
      if (!$scope.settingsFormData.periodic_builds) {
        $scope.settingsFormData.periodic_builds = [];
      }
      $scope.settingsFormData.periodic_builds = $scope.settingsFormData.periodic_builds.concat(
        $scope.periodic_build
      );
      $scope.periodic_build = {};
    };

    $scope.periodicBuildErrors = function () {
      if (!$scope.periodic_build) {
        return "";
      }
      if ($scope.periodic_build.interval_hours <= 0) {
        return "Interval must be a positive integer";
      }
      if ($scope.periodic_build.config_file === "") {
        return "Config file must be defined";
      }
      if ($scope.periodic_build.alias === "") {
        return "Alias file must be defined";
      }
      return "";
    };

    $scope.removeWorkstationCommand = function (i) {
      if (
        $scope.settingsFormData.workstation_config &&
        $scope.settingsFormData.workstation_config.setup_commands[i]
      ) {
        $scope.settingsFormData.workstation_config.setup_commands.splice(i, 1);
        $scope.isDirty = true;
      }
    };

    $scope.triggerLabel = function (trigger) {
      if (!trigger || !trigger.project) {
        return "";
      }
      var out = "When a " + trigger.level;
      out = out + " matching variant '" + (trigger.variant_regex || "*") + "'";
      out = out + " and task '" + (trigger.task_regex || "*") + "'";
      out = out + " in project '" + trigger.project + "'";
      out = out + " finishes with status '" + (trigger.status || "*") + "'";
      out = out + ", run the tasks defined in '" + trigger.file + "'";
      if (trigger.command) {
        out = out + " generated by '" + trigger.command + "'";
      }
      if (trigger.alias) {
        out = out + " filtered to alias '" + trigger.alias + "'";
      }
      return out;
    };

    $scope.periodicBuildLabel = function (definition) {
      if (!definition) {
        return "";
      }
      if (definition.message) {
        return definition.message;
      }
      return `Every ${definition.interval_hours} hours`;
    };

    $scope.$watch("settingsForm.$dirty", function (dirty) {
      if (dirty) {
        $scope.saveMessage = "You have unsaved changes.";
        $scope.isDirty = true;
      }
    });

    $scope.getAlertDisplay = function (alertObj) {
      if (alertObj.provider == "email") {
        return "Send an e-mail to " + alertObj.settings.recipient;
      }
      if (alertObj.provider == "jira") {
        return (
          "File a " +
          alertObj.settings.issue +
          " JIRA ticket in " +
          alertObj.settings.project
        );
      }
      if (alertObj.provider == "slack") {
        return "Send a slack message to " + alertObj.settings.channel;
      }
      return "unknown";
    };

    $scope.removeAlert = function (triggerId, index) {
      $scope.settingsFormData.alert_config[triggerId].splice(index, 1);
      $scope.isDirty = true;
    };

    $scope.isValidMergeBaseRevision = function (revision) {
      return revision && revision.length >= 40;
    };

    $scope.isValidGithubOrg = function (org) {
      // no orgs specified
      if (
        $scope.github_valid_orgs === null ||
        $scope.github_valid_orgs === undefined ||
        $scope.github_valid_orgs.length === 0
      ) {
        return true;
      }
      for (var i = 0; i < $scope.github_valid_orgs.length; i++) {
        if (org === $scope.github_valid_orgs[i]) {
          return true;
        }
      }
      return false;
    };

    $scope.setLastRevision = function () {
      if ($scope.settingsFormData.repotracker_error.exists) {
        var revisionUrl =
          "/project/" + $scope.settingsFormData.identifier + "/repo_revision";
        if (
          !$scope.isValidMergeBaseRevision(
            $scope.settingsFormData.repotracker_error.merge_base_revision
          )
        ) {
          console.log("bad revision");
          return;
        }
        $http
          .put(
            revisionUrl,
            $scope.settingsFormData.repotracker_error.merge_base_revision
          )
          .then(
            function (data) {
              $scope.settingsFormData.repotracker_error.exists = false;
            },
            function (resp) {
              console.log(resp.status);
            }
          );
      }
    };

    $scope.validKeyValue = function (keyName, value) {
      if (!keyName) {
        $scope.invalidKeyMessage = "";
        return false;
      }

      if (keyName.indexOf(".") != -1) {
        $scope.invalidKeyMessage =
          "Project variable keys cannot have dots in them";
        return false;
      }

      if (!value) {
        $scope.invalidKeyMessage = "";
        return false;
      }
      return true;
    };

    $scope.patchDefinitionPopulated = function (alias) {
      return (
        alias &&
        (alias.variant ||
          alias.task ||
          !_.isEmpty(alias.variant_tags) ||
          !_.isEmpty(alias.tags))
      );
    };

    $scope.aliasRemotePathPopulated = function (alias) {
      return Boolean(alias.remote_path);
    };

    $scope.validGitTagVersionDefinition = function (alias) {
      if (!alias || !alias.git_tag) {
        return false;
      }
      // must have ONLY yaml file defined, or a valid patch definition
      if (Boolean(alias.remote_path)) {
        return !$scope.patchDefinitionPopulated(alias);
      }
      return $scope.validPatchDefinition(alias);
    };

    $scope.validPatchDefinition = function (alias) {
      // (variant XOR variant_tags) AND (task XOR tags)
      return (
        alias &&
        Boolean(alias.variant) != !_.isEmpty(alias.variant_tags) &&
        Boolean(alias.task) != !_.isEmpty(alias.tags)
      );
    };

    $scope.validPatchAlias = function (alias) {
      // Same as GitHub alias, but with alias required
      return $scope.validPatchDefinition(alias) && alias.alias;
    };

    $scope.validWorkstationCommand = function (obj) {
      return (
        obj !== undefined && obj.command !== undefined && obj.command !== ""
      );
    };

    $scope.showPatchTriggerAliasModal = function (index) {
      if (index != undefined) {
        var aliasToEdit = $scope.patch_trigger_aliases[index];
      }
      var modal = $mdDialog.confirm({
        title: "New Patch Trigger Alias",
        templateUrl: "/static/partials/patch_trigger_alias_modal.html",
        controllerAs: "data",
        controller: newPatchTriggerAliasController,
        bindToController: true,
        locals: {
          aliasToEdit: aliasToEdit,
          index: index,
        },
      });

      $mdDialog.show(modal).then(function (update) {
        if (update.aliasIndex != undefined) {
          if (update.delete) {
            $scope.patch_trigger_aliases.splice(update.aliasIndex, 1);
          } else {
            $scope.patch_trigger_aliases[update.aliasIndex] = update.alias;
          }
        } else {
          $scope.patch_trigger_aliases = $scope.patch_trigger_aliases.concat([
            update.alias,
          ]);
        }
        $scope.isDirty = true;
      });
    };

    function newPatchTriggerAliasController($scope, $mdDialog) {
      $scope.new_task_specifier = {};
      $scope.alias = {
        task_specifiers: [],
      };

      if ($scope.data.aliasToEdit) {
        $scope.alias = $scope.data.aliasToEdit;
        $scope.alias.task_specifiers = $scope.alias.task_specifiers || [];
        $scope.aliasIndex = $scope.data.index;
      }

      $scope.removeTaskSpecifier = function (index) {
        $scope.alias.task_specifiers.splice(index, 1);
      };

      $scope.addTaskSpecifier = function () {
        if (!$scope.validTaskSpecifier($scope.new_task_specifier)) {
          return;
        }

        $scope.alias.task_specifiers.push($scope.new_task_specifier);
        $scope.new_task_specifier = {};
      };

      $scope.validTaskSpecifier = function (specifier) {
        // can't specify both an alias and a regex set
        if (
          specifier.patch_alias &&
          (specifier.variant_regex || specifier.task_regex)
        ) {
          return false;
        }

        // must specify either an alias or a complete regex set
        if (
          !specifier.patch_alias &&
          (!specifier.variant_regex || !specifier.task_regex)
        ) {
          return false;
        }

        return true;
      };

      $scope.closeDialog = function (save) {
        if (save) {
          if (!$scope.validTriggerAlias()) {
            return;
          }

          $mdDialog.hide({
            alias: $scope.alias,
            aliasIndex: $scope.aliasIndex,
          });
        }
        $mdDialog.cancel();
      };

      $scope.deleteAlias = function () {
        $mdDialog.hide({
          delete: true,
          aliasIndex: $scope.aliasIndex,
        });
      };

      $scope.validTriggerAlias = function () {
        if (!$scope.alias.alias || !$scope.alias.child_project) {
          return false;
        }

        if (!$scope.alias.task_specifiers.length) {
          return false;
        }

        return true;
      };

      $scope.modalTitle = function () {
        if ($scope.data.aliasToEdit) {
          return "Edit Patch Trigger Alias";
        } else {
          return "New Patch Trigger Alias";
        }
      };
    }

    $scope.showTriggerModal = function (index) {
      if (index != undefined) {
        var triggerToEdit = $scope.project_triggers[index];
        if (!triggerToEdit.status) {
          triggerToEdit.status = "all";
        }
      }
      var modal = $mdDialog.confirm({
        title: "New Trigger",
        templateUrl: "/static/partials/project_trigger_modal.html",
        controllerAs: "data",
        controller: newTriggerController,
        bindToController: true,
        locals: {
          triggerToEdit: triggerToEdit,
          index: index,
        },
      });

      $mdDialog.show(modal).then(function (update) {
        if (update.triggerIndex != undefined) {
          if (update.delete) {
            $scope.project_triggers.splice(update.triggerIndex, 1);
          } else {
            $scope.project_triggers[update.triggerIndex] = update.trigger;
          }
        } else {
          $scope.project_triggers = $scope.project_triggers.concat([
            update.trigger,
          ]);
        }
        $scope.isDirty = true;
      });
    };

    function newTriggerController($scope, $mdDialog) {
      if ($scope.data.triggerToEdit) {
        $scope.trigger = $scope.data.triggerToEdit;
        $scope.triggerIndex = $scope.data.index;
      } else {
        $scope.trigger = {
          level: "task",
          status: "all",
        };
      }
      $scope.closeDialog = function (save) {
        if (save) {
          if (!$scope.validProjectTrigger()) {
            return;
          }
          if ($scope.trigger.status === "all") {
            // workaround for https://github.com/angular/material/issues/9178
            $scope.trigger.status = "";
          }
          $mdDialog.hide({
            trigger: $scope.trigger,
            triggerIndex: $scope.triggerIndex,
          });
        }
        $mdDialog.cancel();
      };

      $scope.deleteTrigger = function () {
        $mdDialog.hide({
          delete: true,
          triggerIndex: $scope.triggerIndex,
        });
      };

      $scope.validProjectTrigger = function () {
        if (
          !$scope.trigger.project ||
          !$scope.trigger.level ||
          !$scope.trigger.file
        ) {
          return false;
        }

        return true;
      };

      $scope.modalTitle = function () {
        if ($scope.data.triggerToEdit) {
          return "Edit Trigger";
        } else {
          return "New Trigger";
        }
      };
    }

    function comebineDateTime(date, time) {
      date.setHours(time.getHours());
      date.setMinutes(time.getMinutes());

      return date;
    }

    $scope.showPeriodicBuildModal = function (index) {
      var modal = $mdDialog.confirm({
        title: "Edit Periodic Build Definition",
        templateUrl: "/static/partials/periodic_build_modal.html",
        controllerAs: "data",
        controller: newPeriodicBuildController,
        bindToController: true,
        locals: {
          periodicBuildToEdit: $scope.periodic_builds[index],
          index: index,
          timezones: timeUtil.timezones,
          userTz: window.userTz,
        },
      });

      $mdDialog.show(modal).then(function (update) {
        if (update.delete) {
          $scope.periodic_builds.splice(update.periodicBuildIndex, 1);
        } else {
          update.periodic_build.next_run_time = comebineDateTime(
            update.periodic_build.start_date,
            update.periodic_build.start_time
          );
          // annoying way to set the time zone of the date to the user-specified one
          let offset =
            moment.tz(window.userTz).utcOffset() -
            moment.tz(update.periodic_build.timezone).utcOffset();
          update.periodic_build.next_run_time.setMinutes(
            update.periodic_build.next_run_time.getMinutes() + offset
          );
          if (update.periodicBuildIndex !== undefined) {
            $scope.periodic_builds[update.periodicBuildIndex] =
              update.periodic_build;
          } else {
            $scope.periodic_builds = $scope.periodic_builds.concat([
              update.periodic_build,
            ]);
          }
        }
        $scope.isDirty = true;
      });
    };

    function newPeriodicBuildController($scope, $mdDialog) {
      if ($scope.data.periodicBuildToEdit) {
        $scope.periodic_build = $scope.data.periodicBuildToEdit;
        $scope.periodicBuildIndex = $scope.data.index;
      } else {
        $scope.periodic_build = {};
      }
      if ($scope.periodic_build.next_run_time) {
        if (typeof $scope.periodic_build.next_run_time === "string") {
          $scope.periodic_build.next_run_time = new Date(
            $scope.periodic_build.next_run_time
          );
        }
        $scope.periodic_build.start_date = $scope.periodic_build.next_run_time; // TODO: this deserializes as string
        $scope.periodic_build.start_time = $scope.periodic_build.next_run_time;
      } else {
        $scope.periodic_build.start_date = new Date();
        $scope.periodic_build.start_time = new Date();
      }
      if ($scope.data.userTz) {
        $scope.periodic_build.timezone = $scope.data.userTz;
      }

      $scope.closeDialog = function (save) {
        if (save) {
          $mdDialog.hide({
            periodic_build: $scope.periodic_build,
            periodicBuildIndex: $scope.periodicBuildIndex,
          });
        }
        $mdDialog.cancel();
      };

      $scope.deleteDefinition = function () {
        $mdDialog.hide({
          delete: true,
          periodicBuildIndex: $scope.periodicBuildIndex,
        });
      };

      $scope.modalTitle = function () {
        if ($scope.data.periodic_build) {
          return "Edit Periodic Build";
        } else {
          return "New Periodic Build";
        }
      };
    }

    $scope.addSubscription = function () {
      promise = addSubscriber($mdDialog, $scope.triggers);

      $mdDialog.show(promise).then(function (data) {
        data.changed = true;
        $scope.isDirty = true;
        $scope.subscriptions.push(data);
      });
    };

    $scope.editSubscription = function (index) {
      promise = editSubscriber(
        $mdDialog,
        $scope.triggers,
        $scope.subscriptions[index]
      );

      $mdDialog.show(promise).then(function (data) {
        data.changed = true;
        $scope.isDirty = true;
        $scope.subscriptions[index] = data;
      });
    };

    $scope.removeSubscription = function (index) {
      if ($scope.subscriptions[index] && $scope.subscriptions[index].id) {
        $scope.settingsFormData.delete_subscriptions.push(
          $scope.subscriptions[index].id
        );
      }
      $scope.subscriptions = _.filter($scope.subscriptions, function (s, i) {
        return index !== i;
      });
      $scope.isDirty = true;
    };

    $scope.show_build_break = true;
    $scope.validMergeMethods = ["squash", "merge", "rebase"];
  }
);

mciModule.directive("adminNewProject", function () {
  return {
    restrict: "E",
    template: '' +
      '<div class="row">' +
        '<div class="col-lg-12">' +
         "Enter project identifier " +
          '<input type="text" id="project-name" placeholder="project name" ng-model="newProject.identifier">' +
        "</div>" +
      '</div>' +
      '<div class="row">' +
        '<div class="col-lg-12">' +
          "Optionally enter immutable project ID " +
          '<div class="muted small">' +
            "(Used by Evergreen internally and defaults to a random hash; should only be user-specified with good reason. Cannot be changed!)" +
          '</div>' +
          '<input type="text" id="project-name" placeholder="immutable project ID" ng-model="newProject.id">' +
        '</div>' +
      '</div>' +
      '<div class="row">' +
        '<div class="col-lg-12">' +
        '<form style="display: inline">' +
          '<input type="checkbox" id="copy-project" ng-model="newProject.copyProject">' +
          " Duplicate current project " +
        "</form>" +
        '</div>' +
      '</div>' +
      '<div class="row">'  +
        '<div class="col-lg-12">' +
          '<button type="submit" class="btn btn-primary" style="float: right; margin-left: 10px;" ng-disabled="!newProject.identifier" ng-click="addProject()">Create Project</button>' +
        '</div>' +
      '</div>',
  };
});
