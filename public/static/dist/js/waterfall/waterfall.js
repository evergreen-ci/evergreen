"use strict";

var _createClass = function () { function defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } } return function (Constructor, protoProps, staticProps) { if (protoProps) defineProperties(Constructor.prototype, protoProps); if (staticProps) defineProperties(Constructor, staticProps); return Constructor; }; }();

function _defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

function _classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function _possibleConstructorReturn(self, call) { if (!self) { throw new ReferenceError("this hasn't been initialised - super() hasn't been called"); } return call && (typeof call === "object" || typeof call === "function") ? call : self; }

function _inherits(subClass, superClass) { if (typeof superClass !== "function" && superClass !== null) { throw new TypeError("Super expression must either be null or a function, not " + typeof superClass); } subClass.prototype = Object.create(superClass && superClass.prototype, { constructor: { value: subClass, enumerable: false, writable: true, configurable: true } }); if (superClass) Object.setPrototypeOf ? Object.setPrototypeOf(subClass, superClass) : subClass.__proto__ = superClass; }

/*
 ReactJS code for the Waterfall page. Grid calls the Variant class for each distro, and the Variant class renders each build variant for every version that exists. In each build variant we iterate through all the tasks and render them as well. The row of headers is just a placeholder at the moment.
*/

// React doesn't provide own functionality for making http calls
// window.fetch doesn't handle query params
var http = angular.bootstrap().get('$http');

// Returns string from datetime object in "5/7/96 1:15 AM" format
// Used to display version headers
function getFormattedTime(input, userTz, fmt) {
  return moment(input).tz(userTz).format(fmt);
}

function gitTagsMessage(git_tags) {
  if (!git_tags || git_tags === "") {
    return "";
  }
  return "Git Tags: " + git_tags + "";
}

function generateURLParameters(params) {
  var ret = [];
  for (var p in params) {
    ret.push(encodeURIComponent(p) + "=" + encodeURIComponent(params[p]));
  }
  return ret.join("&");
}

// getParameterByName returns the value associated with a given query parameter.
// Based on: http://stackoverflow.com/questions/901115/how-can-i-get-query-string-values-in-javascript
function getParameterByName(name, url) {
  name = name.replace(/[\[\]]/g, "\\$&");
  var regex = new RegExp("[?&]" + name + "(=([^&#]*)|&|#|$)");
  var results = regex.exec(url);
  if (!results) {
    return null;
  }
  if (!results[2]) {
    return '';
  }
  return decodeURIComponent(results[2].replace(/\+/g, " "));
}

function updateURLParams(bvFilter, taskFilter, skip, baseURL, showUpstream) {
  var params = {};
  if (bvFilter && bvFilter != '') params["bv_filter"] = bvFilter;
  if (taskFilter && taskFilter != '') params["task_filter"] = taskFilter;
  if (skip !== 0) {
    params["skip"] = skip;
  }
  if (showUpstream !== undefined) {
    params["upstream"] = showUpstream;
  }

  if (Object.keys(params).length > 0) {
    var paramString = generateURLParameters(params);
    window.history.replaceState({}, '', baseURL + "?" + paramString);
  } else {
    window.history.replaceState({}, '', baseURL);
  }
}

var JIRA_REGEX = /[A-Z]{1,10}-\d{1,6}/ig;

var JiraLink = function (_React$PureComponent) {
  _inherits(JiraLink, _React$PureComponent);

  function JiraLink() {
    _classCallCheck(this, JiraLink);

    return _possibleConstructorReturn(this, (JiraLink.__proto__ || Object.getPrototypeOf(JiraLink)).apply(this, arguments));
  }

  _createClass(JiraLink, [{
    key: "render",
    value: function render() {
      var contents;

      if (_.isString(this.props.children)) {
        var tokens = this.props.children.split(/\s/);
        var jiraHost = this.props.jiraHost;

        contents = _.map(tokens, function (token, i) {
          var hasSpace = i !== tokens.length - 1;
          var maybeSpace = hasSpace ? ' ' : '';
          var capture = '';
          if (capture = token.match(JIRA_REGEX)) {
            var jiraLink = "https://" + jiraHost + "/browse/" + capture;
            return React.createElement(
              "a",
              { href: jiraLink },
              token + maybeSpace
            );
          } else {
            return token + maybeSpace;
          }
        });
      } else {
        return null;
      }
      return React.createElement(
        "div",
        null,
        contents
      );
    }
  }]);

  return JiraLink;
}(React.PureComponent);

// The Root class renders all components on the waterfall page, including the grid view and the filter and new page buttons
// The one exception is the header, which is written in Angular and managed by menu.html


var Root = function (_React$PureComponent2) {
  _inherits(Root, _React$PureComponent2);

  function Root(props) {
    _classCallCheck(this, Root);

    var _this2 = _possibleConstructorReturn(this, (Root.__proto__ || Object.getPrototypeOf(Root)).call(this, props));

    var href = window.location.href;
    var buildVariantFilter = getParameterByName('bv_filter', href) || '';
    var taskFilter = getParameterByName('task_filter', href) || '';

    var collapsed = localStorage.getItem("collapsed") == "true";
    var showUpstream = localStorage.getItem("show_upstream") == "true";

    _this2.state = {
      collapsed: collapsed,
      shortenCommitMessage: true,
      buildVariantFilter: buildVariantFilter,
      taskFilter: taskFilter,
      showUpstream: showUpstream,
      data: null
    };
    _this2.nextSkip = getParameterByName('skip', href) || 0;
    _this2.baseURL = "/waterfall/" + _this2.props.project;

    // Handle state for a collapsed view, as well as shortened header commit messages
    _this2.handleCollapseChange = _this2.handleCollapseChange.bind(_this2);
    _this2.onToggleShowUpstream = _this2.onToggleShowUpstream.bind(_this2);
    _this2.handleHeaderLinkClick = _this2.handleHeaderLinkClick.bind(_this2);
    _this2.handleBuildVariantFilter = _this2.handleBuildVariantFilter.bind(_this2);
    _this2.handleTaskFilter = _this2.handleTaskFilter.bind(_this2);
    _this2.loadDataPortion = _this2.loadDataPortion.bind(_this2);
    _this2.loadDataPortion();
    _this2.loadDataPortion = _.debounce(_this2.loadDataPortion, 1000);
    _this2.loadData = _this2.loadData.bind(_this2);
    return _this2;
  }

  _createClass(Root, [{
    key: "updatePaginationContext",
    value: function updatePaginationContext(data) {
      // Initialize newer|older buttons
      var versionsOnPage = _.reduce(data.versions, function (m, d) {
        return m + d.authors.length;
      }, 0);

      this.currentSkip = data.current_skip;
      this.nextSkip = this.currentSkip + versionsOnPage;
      this.prevSkip = this.currentSkip - data.previous_page_count;

      if (this.nextSkip >= data.total_versions) {
        this.nextSkip = -1;
      }

      if (this.currentSkip <= 0) {
        this.prevSkip = -1;
      }
    }
  }, {
    key: "loadData",
    value: function loadData(direction) {
      var skip = direction === -1 ? this.prevSkip : this.nextSkip;
      this.loadDataPortion(undefined, skip);
    }
  }, {
    key: "loadDataPortion",
    value: function loadDataPortion(filter, skip) {
      var _this3 = this;

      var params = {
        skip: skip === undefined ? this.nextSkip : skip
      };
      if (this.state.buildVariantFilter) {
        params.bv_filter = this.state.buildVariantFilter;
      }
      if (filter !== undefined && filter !== this.state.buildVariantFilter) {
        params.bv_filter = filter;
        params.skip = 0;
      }
      if (this.state.showUpstream) {
        params.upstream = true;
      }
      if (params.skip === -1) {
        delete params.skip;
      }
      this.setState({ data: null });
      http.get("/rest/v1/waterfall/" + this.props.project, { params: params }).then(function (_ref) {
        var data = _ref.data;

        _this3.updatePaginationContext(data);
        _this3.setState({ data: data, nextSkip: _this3.nextSkip + data.versions.length });
        updateURLParams(params.bv_filter, _this3.state.taskFilter, _this3.currentSkip, _this3.baseURL);
      });
    }
  }, {
    key: "handleCollapseChange",
    value: function handleCollapseChange(collapsed) {
      localStorage.setItem("collapsed", collapsed);
      this.setState({ collapsed: collapsed });
    }
  }, {
    key: "handleBuildVariantFilter",
    value: function handleBuildVariantFilter(filter) {
      this.loadDataPortion(filter, this.currentSkip);
      updateURLParams(filter, this.state.taskFilter, this.currentSkip, this.baseURL);
      this.setState({ buildVariantFilter: filter });
    }
  }, {
    key: "handleTaskFilter",
    value: function handleTaskFilter(filter) {
      updateURLParams(this.state.buildVariantFilter, filter, this.currentSkip, this.baseURL);
      this.setState({ taskFilter: filter });
    }
  }, {
    key: "handleHeaderLinkClick",
    value: function handleHeaderLinkClick(shortenMessage) {
      this.setState({ shortenCommitMessage: !shortenMessage });
    }
  }, {
    key: "onToggleShowUpstream",
    value: function onToggleShowUpstream() {
      var showUpstream = !this.state.showUpstream;
      this.loadDataPortion(this.state.buildVariantFilter, this.currentSkip);
      localStorage.setItem("show_upstream", showUpstream);
      updateURLParams(this.state.buildVariantFilter, this.state.taskFilter, this.currentSkip, this.baseURL, showUpstream);
      this.setState({ showUpstream: showUpstream });
    }
  }, {
    key: "render",
    value: function render() {
      if (this.state.data && this.state.data.rows.length == 0) {
        return React.createElement(
          "div",
          null,
          "There are no builds for this project."
        );
      }
      var collapseInfo = {
        collapsed: this.state.collapsed,
        activeTaskStatuses: ['failed', 'system-failed']
      };
      return React.createElement(
        "div",
        null,
        React.createElement(Toolbar, {
          collapsed: this.state.collapsed,
          onCheckCollapsed: this.handleCollapseChange,
          baseURL: this.baseURL,
          nextSkip: this.nextSkip,
          prevSkip: this.prevSkip,
          buildVariantFilter: this.state.buildVariantFilter,
          taskFilter: this.state.taskFilter,
          buildVariantFilterFunc: this.handleBuildVariantFilter,
          taskFilterFunc: this.handleTaskFilter,
          isLoggedIn: this.props.user !== null,
          project: this.props.project,
          disabled: false,
          loadData: this.loadData,
          onToggleShowUpstream: this.onToggleShowUpstream,
          showUpstream: this.state.showUpstream
        }),
        React.createElement(Headers, {
          shortenCommitMessage: this.state.shortenCommitMessage,
          versions: this.state.data === null ? null : this.state.data.versions,
          onLinkClick: this.handleHeaderLinkClick,
          userTz: this.props.userTz,
          jiraHost: this.props.jiraHost
        }),
        React.createElement(Grid, {
          data: this.state.data,
          collapseInfo: collapseInfo,
          project: this.props.project,
          buildVariantFilter: this.state.buildVariantFilter,
          taskFilter: this.state.taskFilter
        })
      );
    }
  }]);

  return Root;
}(React.PureComponent);

// Toolbar


function Toolbar(_ref2) {
  var collapsed = _ref2.collapsed,
      onCheckCollapsed = _ref2.onCheckCollapsed,
      baseURL = _ref2.baseURL,
      nextSkip = _ref2.nextSkip,
      prevSkip = _ref2.prevSkip,
      buildVariantFilter = _ref2.buildVariantFilter,
      taskFilter = _ref2.taskFilter,
      buildVariantFilterFunc = _ref2.buildVariantFilterFunc,
      taskFilterFunc = _ref2.taskFilterFunc,
      isLoggedIn = _ref2.isLoggedIn,
      project = _ref2.project,
      disabled = _ref2.disabled,
      loadData = _ref2.loadData,
      showUpstream = _ref2.showUpstream,
      onToggleShowUpstream = _ref2.onToggleShowUpstream;


  var Form = ReactBootstrap.Form;
  return React.createElement(
    "div",
    { className: "row" },
    React.createElement(
      "div",
      { className: "col-xs-12" },
      React.createElement(
        Form,
        { inline: true, className: "waterfall-toolbar pull-right" },
        React.createElement(CollapseButton, { collapsed: collapsed, onCheckCollapsed: onCheckCollapsed, disabled: disabled }),
        React.createElement(FilterBox, {
          filterFunction: buildVariantFilterFunc,
          placeholder: "Filter variant",
          currentFilter: buildVariantFilter,
          disabled: disabled
        }),
        React.createElement(FilterBox, {
          filterFunction: taskFilterFunc,
          placeholder: "Filter task",
          currentFilter: taskFilter,
          disabled: collapsed || disabled
        }),
        React.createElement(PageButtons, {
          nextSkip: nextSkip,
          prevSkip: prevSkip,
          baseURL: baseURL,
          buildVariantFilter: buildVariantFilter,
          taskFilter: taskFilter,
          disabled: disabled,
          loadData: loadData
        }),
        React.createElement(GearMenu, {
          project: project,
          isLoggedIn: isLoggedIn,
          showUpstream: showUpstream,
          onToggleShowUpstream: onToggleShowUpstream
        })
      )
    )
  );
};

var PageButtons = function (_React$PureComponent3) {
  _inherits(PageButtons, _React$PureComponent3);

  function PageButtons(props) {
    _classCallCheck(this, PageButtons);

    var _this4 = _possibleConstructorReturn(this, (PageButtons.__proto__ || Object.getPrototypeOf(PageButtons)).call(this, props));

    _this4.loadNext = function () {
      return _this4.props.loadData(1);
    };
    _this4.loadPrev = function () {
      return _this4.props.loadData(-1);
    };
    return _this4;
  }

  _createClass(PageButtons, [{
    key: "render",
    value: function render() {
      var ButtonGroup = ReactBootstrap.ButtonGroup;
      return React.createElement(
        "span",
        { className: "waterfall-form-item" },
        React.createElement(
          ButtonGroup,
          null,
          React.createElement(PageButton, { disabled: this.props.disabled || this.props.prevSkip < 0, directionIcon: "fa-chevron-left", loadData: this.loadPrev }),
          React.createElement(PageButton, { disabled: this.props.disabled || this.props.nextSkip < 0, directionIcon: "fa-chevron-right", loadData: this.loadNext })
        )
      );
    }
  }]);

  return PageButtons;
}(React.PureComponent);

function PageButton(_ref3) {
  var directionIcon = _ref3.directionIcon,
      disabled = _ref3.disabled,
      loadData = _ref3.loadData;

  var Button = ReactBootstrap.Button;
  var classes = "fa " + directionIcon;
  return React.createElement(
    Button,
    { onClick: loadData, disabled: disabled },
    React.createElement("i", { className: classes })
  );
}

var FilterBox = function (_React$PureComponent4) {
  _inherits(FilterBox, _React$PureComponent4);

  function FilterBox(props) {
    _classCallCheck(this, FilterBox);

    var _this5 = _possibleConstructorReturn(this, (FilterBox.__proto__ || Object.getPrototypeOf(FilterBox)).call(this, props));

    _this5.applyFilter = _this5.applyFilter.bind(_this5);
    return _this5;
  }

  _createClass(FilterBox, [{
    key: "applyFilter",
    value: function applyFilter() {
      this.props.filterFunction(this.refs.searchInput.value);
    }
  }, {
    key: "render",
    value: function render() {
      return React.createElement("input", { type: "text", ref: "searchInput",
        className: "form-control waterfall-form-item",
        placeholder: this.props.placeholder,
        value: this.props.currentFilter, onChange: this.applyFilter,
        disabled: this.props.disabled });
    }
  }]);

  return FilterBox;
}(React.PureComponent);

var CollapseButton = function (_React$PureComponent5) {
  _inherits(CollapseButton, _React$PureComponent5);

  function CollapseButton(props) {
    _classCallCheck(this, CollapseButton);

    var _this6 = _possibleConstructorReturn(this, (CollapseButton.__proto__ || Object.getPrototypeOf(CollapseButton)).call(this, props));

    _this6.handleChange = _this6.handleChange.bind(_this6);
    return _this6;
  }

  _createClass(CollapseButton, [{
    key: "handleChange",
    value: function handleChange(event) {
      this.props.onCheckCollapsed(this.refs.collapsedBuilds.checked);
    }
  }, {
    key: "render",
    value: function render() {
      return React.createElement(
        "span",
        { className: "semi-muted waterfall-form-item" },
        React.createElement(
          "span",
          { id: "collapsed-prompt" },
          "Show collapsed view"
        ),
        React.createElement("input", {
          className: "checkbox waterfall-checkbox",
          type: "checkbox",
          checked: this.props.collapsed,
          ref: "collapsedBuilds",
          onChange: this.handleChange,
          disabled: this.props.disabled
        })
      );
    }
  }]);

  return CollapseButton;
}(React.PureComponent);

var GearMenu = function (_React$PureComponent6) {
  _inherits(GearMenu, _React$PureComponent6);

  function GearMenu(props) {
    _classCallCheck(this, GearMenu);

    var _this7 = _possibleConstructorReturn(this, (GearMenu.__proto__ || Object.getPrototypeOf(GearMenu)).call(this, props));

    _this7.addNotification = _this7.addNotification.bind(_this7);
    var requesterSubscriberSettings = { text: "Build initiator", key: "requester", type: "select", options: {
        "gitter_request": "Commit",
        "patch_request": "Patch",
        "github_pull_request": "Pull Request",
        "merge_test": "Commit Queue",
        "ad_hoc": "Periodic Build"
      }, default: "gitter_request" };
    _this7.triggers = [{
      trigger: "outcome",
      resource_type: "TASK",
      label: "any task finishes",
      regex_selectors: taskRegexSelectors(),
      extraFields: [requesterSubscriberSettings]
    }, {
      trigger: "failure",
      resource_type: "TASK",
      label: "any task fails",
      regex_selectors: taskRegexSelectors(),
      extraFields: [{ text: "Failure type", key: "failure-type", type: "select", options: { "any": "Any", "test": "Test", "system": "System", "setup": "Setup" }, default: "any" }, requesterSubscriberSettings]
    }, {
      trigger: "success",
      resource_type: "TASK",
      label: "any task succeeds",
      regex_selectors: taskRegexSelectors(),
      extraFields: [requesterSubscriberSettings]
    }, {
      trigger: "exceeds-duration",
      resource_type: "TASK",
      label: "the runtime for any task exceeds some duration",
      extraFields: [{ text: "Task duration (seconds)", key: "task-duration-secs", validator: validateDuration }],
      regex_selectors: taskRegexSelectors()
    }, {
      trigger: "runtime-change",
      resource_type: "TASK",
      label: "the runtime for a successful task changes by some percentage",
      extraFields: [{ text: "Percent change", key: "task-percent-change", validator: validatePercentage }],
      regex_selectors: taskRegexSelectors()
    }, {
      trigger: "outcome",
      resource_type: "BUILD",
      label: "a build-variant in any version finishes",
      regex_selectors: buildRegexSelectors(),
      extraFields: [requesterSubscriberSettings]
    }, {
      trigger: "failure",
      resource_type: "BUILD",
      label: "a build-variant in any version fails",
      regex_selectors: buildRegexSelectors(),
      extraFields: [requesterSubscriberSettings]
    }, {
      trigger: "success",
      resource_type: "BUILD",
      label: "a build-variant in any version succeeds",
      regex_selectors: buildRegexSelectors(),
      extraFields: [requesterSubscriberSettings]
    }, {
      trigger: "outcome",
      resource_type: "VERSION",
      label: "any version finishes",
      extraFields: [requesterSubscriberSettings]
    }, {
      trigger: "failure",
      resource_type: "VERSION",
      label: "any version fails",
      extraFields: [requesterSubscriberSettings]
    }, {
      trigger: "success",
      resource_type: "VERSION",
      label: "any version succeeds",
      extraFields: [requesterSubscriberSettings]
    }];
    return _this7;
  }

  _createClass(GearMenu, [{
    key: "dialog",
    value: function dialog($mdDialog, $mdToast, notificationService, mciSubscriptionsService) {
      var _omitMethods;

      var omitMethods = (_omitMethods = {}, _defineProperty(_omitMethods, SUBSCRIPTION_JIRA_ISSUE, true), _defineProperty(_omitMethods, SUBSCRIPTION_EVERGREEN_WEBHOOK, true), _omitMethods);

      var self = this;
      var promise = addSubscriber($mdDialog, this.triggers, omitMethods);
      return $mdDialog.show(promise).then(function (data) {
        addProjectSelectors(data, self.project);
        var success = function success() {
          return $mdToast.show({
            templateUrl: "/static/partials/subscription_confirmation_toast.html",
            position: "bottom right"
          });
        };
        var failure = function failure(resp) {
          notificationService.pushNotification('Error saving subscriptions: ' + resp.data.error, 'errorHeader');
        };
        mciSubscriptionsService.post([data], { success: success, error: failure });
      }).catch(function (e) {
        notificationService.pushNotification('Error saving subscriptions: ' + e, 'errorHeader');
      });
    }
  }, {
    key: "addNotification",
    value: function addNotification() {
      var waterfall = angular.module('waterfall', ['ng', 'MCI', 'material.components.toast']);
      waterfall.provider({
        $rootElement: function $rootElement() {
          this.$get = function () {
            var root = document.getElementById("root");
            return angular.element(root);
          };
        }
      });

      var injector = angular.injector(['waterfall']);
      return injector.invoke(this.dialog, { triggers: this.triggers, project: this.props.project });
    }
  }, {
    key: "render",
    value: function render() {
      if (!this.props.isLoggedIn) {
        return null;
      }
      var ButtonGroup = ReactBootstrap.ButtonGroup;
      var Button = ReactBootstrap.Button;
      var DropdownButton = ReactBootstrap.DropdownButton;
      var MenuItem = ReactBootstrap.MenuItem;

      return React.createElement(
        "span",
        null,
        React.createElement(
          DropdownButton,
          { className: "fa fa-gear", pullRight: true, id: "waterfall-gear-menu" },
          React.createElement(
            MenuItem,
            { onClick: this.addNotification },
            "Add Notification"
          ),
          React.createElement(
            MenuItem,
            { onClick: this.props.onToggleShowUpstream },
            this.props.showUpstream ? "Hide Upstream Commits" : "Show Upstream Commits"
          )
        )
      );
    }
  }]);

  return GearMenu;
}(React.PureComponent);

;

// Headers

function Headers(_ref4) {
  var shortenCommitMessage = _ref4.shortenCommitMessage,
      versions = _ref4.versions,
      onLinkClick = _ref4.onLinkClick,
      userTz = _ref4.userTz,
      jiraHost = _ref4.jiraHost;

  if (versions === null) {
    return React.createElement(VersionHeaderTombstone, null);
  }
  return React.createElement(
    "div",
    { className: "row version-header" },
    React.createElement("div", { className: "variant-col col-xs-2 version-header-rolled" }),
    React.createElement(
      "div",
      { className: "col-xs-10" },
      React.createElement(
        "div",
        { className: "row" },
        versions.map(function (version) {
          if (version.rolled_up) {
            return React.createElement(RolledUpVersionHeader, {
              key: version.ids[0],
              version: version,
              userTz: userTz,
              jiraHost: jiraHost
            });
          }
          // Unrolled up version, no popover
          return React.createElement(ActiveVersionHeader, {
            key: version.ids[0],
            version: version,
            userTz: userTz,
            shortenCommitMessage: shortenCommitMessage,
            onLinkClick: onLinkClick,
            jiraHost: jiraHost
          });
        })
      )
    )
  );
}

function VersionHeaderTombstone() {
  return React.createElement(
    "div",
    { className: "row version-header" },
    React.createElement("div", { className: "variant-col col-xs-2 version-header-rolled" }),
    React.createElement(
      "div",
      { className: "col-xs-10" },
      React.createElement(
        "div",
        { className: "row" },
        React.createElement(
          "div",
          { className: "header-col" },
          React.createElement(
            "div",
            { className: "version-header-expanded" },
            React.createElement(
              "div",
              { className: "col-xs-12" },
              React.createElement(
                "div",
                { className: "row" },
                React.createElement(
                  "div",
                  { className: "waterfall-tombstone", style: { 'height': '14px', 'width': '126px' } },
                  "\xA0"
                )
              )
            ),
            React.createElement(
              "div",
              { className: "col-xs-12" },
              React.createElement(
                "div",
                { className: "row" },
                React.createElement(
                  "div",
                  { className: "waterfall-tombstone", style: { 'marginTop': '5px', 'height': '14px', 'width': '78px' } },
                  "\xA0"
                ),
                React.createElement(
                  "div",
                  { className: "waterfall-tombstone", style: { 'marginTop': '5px', 'height': '14px', 'width': '205px' } },
                  "\xA0"
                )
              )
            )
          )
        )
      )
    )
  );
}

function ActiveVersionHeader(_ref5) {
  var shortenCommitMessage = _ref5.shortenCommitMessage,
      version = _ref5.version,
      onLinkClick = _ref5.onLinkClick,
      userTz = _ref5.userTz,
      jiraHost = _ref5.jiraHost;

  var message = version.messages[0];
  var author = version.authors[0];
  var id_link = "/version/" + version.ids[0];
  var commit = version.revisions[0].substring(0, 5);
  var tags = gitTagsMessage(version.git_tags[0]);

  if (version.upstream_data) {
    var upstreamData = version.upstream_data[0];
    var upstreamLink = "/" + upstreamData.trigger_type + "/" + upstreamData.trigger_id;
    var upstreamAnchor = React.createElement(
      "a",
      { href: upstreamLink },
      upstreamData.project_name
    );
    var upstreamElem = React.createElement(
      "div",
      { className: "row" },
      " From ",
      upstreamAnchor,
      " "
    );
  }
  var formatted_time = getFormattedTime(version.create_times[0], userTz, 'M/D/YY h:mm A');
  var maxChars = 44;
  var button;
  if (message.length > maxChars) {
    // If we shorten the commit message, only display the first maxChars chars
    if (shortenCommitMessage) {
      message = message.substring(0, maxChars - 3) + "...";
    }
    button = React.createElement(HideHeaderButton, { onLinkClick: onLinkClick, shortenCommitMessage: shortenCommitMessage });
  }

  return React.createElement(
    "div",
    { className: "header-col" },
    React.createElement(
      "div",
      { className: "version-header-expanded" },
      React.createElement(
        "div",
        { className: "col-xs-12" },
        React.createElement(
          "div",
          { className: "row" },
          React.createElement(
            "a",
            { className: "githash", href: id_link },
            commit
          ),
          formatted_time
        ),
        upstreamElem
      ),
      React.createElement(
        "div",
        { className: "col-xs-12" },
        React.createElement(
          "div",
          { className: "row" },
          React.createElement(
            "strong",
            null,
            author
          ),
          " - ",
          React.createElement(
            JiraLink,
            { jiraHost: jiraHost },
            message
          ),
          button,
          React.createElement(
            "div",
            null,
            tags
          )
        )
      )
    )
  );
};

var HideHeaderButton = function (_React$Component) {
  _inherits(HideHeaderButton, _React$Component);

  function HideHeaderButton(props) {
    _classCallCheck(this, HideHeaderButton);

    var _this8 = _possibleConstructorReturn(this, (HideHeaderButton.__proto__ || Object.getPrototypeOf(HideHeaderButton)).call(this, props));

    _this8.onLinkClick = _this8.onLinkClick.bind(_this8);
    return _this8;
  }

  _createClass(HideHeaderButton, [{
    key: "onLinkClick",
    value: function onLinkClick(event) {
      this.props.onLinkClick(this.props.shortenCommitMessage);
    }
  }, {
    key: "render",
    value: function render() {
      var textToShow = this.props.shortenCommitMessage ? "more" : "less";
      return React.createElement(
        "span",
        { onClick: this.onLinkClick },
        " ",
        React.createElement(
          "a",
          { href: "#" },
          textToShow
        ),
        " "
      );
    }
  }]);

  return HideHeaderButton;
}(React.Component);

function RolledUpVersionHeader(_ref6) {
  var version = _ref6.version,
      userTz = _ref6.userTz,
      jiraHost = _ref6.jiraHost;

  var Popover = ReactBootstrap.Popover;
  var OverlayTrigger = ReactBootstrap.OverlayTrigger;
  var Button = ReactBootstrap.Button;

  var versionStr = version.messages.length > 1 ? "versions" : "version";
  var rolledHeader = version.messages.length + " inactive " + versionStr;
  var popovers = React.createElement(
    Popover,
    { id: "popover-positioned-bottom", title: "" },
    version.ids.map(function (id, i) {
      return React.createElement(RolledUpVersionSummary, {
        author: version.authors[i],
        commit: version.revisions[i],
        message: version.messages[i],
        gitTags: version.git_tags[i],
        versionId: version.ids[i],
        key: id, userTz: userTz,
        createTime: version.create_times[i],
        jiraHost: jiraHost });
    })
  );

  return React.createElement(
    "div",
    { className: "header-col version-header-rolled" },
    React.createElement(
      OverlayTrigger,
      { trigger: "click", placement: "bottom", overlay: popovers, className: "col-xs-2" },
      React.createElement(
        "span",
        { className: "pointer" },
        " ",
        rolledHeader,
        " "
      )
    )
  );
};
function RolledUpVersionSummary(_ref7) {
  var author = _ref7.author,
      commit = _ref7.commit,
      message = _ref7.message,
      gitTags = _ref7.gitTags,
      versionId = _ref7.versionId,
      createTime = _ref7.createTime,
      userTz = _ref7.userTz,
      jiraHost = _ref7.jiraHost;

  var formatted_time = getFormattedTime(new Date(createTime), userTz, 'M/D/YY h:mm A');
  gitTags = gitTagsMessage(gitTags);
  commit = commit.substring(0, 10);
  return React.createElement(
    "div",
    { className: "rolled-up-version-summary" },
    React.createElement(
      "span",
      { className: "version-header-time" },
      formatted_time
    ),
    React.createElement("br", null),
    React.createElement(
      "a",
      { href: "/version/" + versionId },
      commit
    ),
    " - ",
    React.createElement(
      "strong",
      null,
      author
    ),
    React.createElement("br", null),
    React.createElement(
      JiraLink,
      { jiraHost: jiraHost },
      message
    ),
    React.createElement(
      "div",
      null,
      gitTags
    ),
    React.createElement("br", null)
  );
}

function TaskTombstones(num) {
  var out = [];
  for (var i = 0; i < num; ++i) {
    out.push(React.createElement("a", { className: "waterfall-box inactive" }));
  }
  return out;
}

function VariantTombstone() {
  return React.createElement(
    "div",
    { className: "row variant-row" },
    React.createElement(
      "div",
      { className: "col-xs-2 build-variants" },
      React.createElement(
        "div",
        { className: "waterfall-tombstone", style: { 'height': '18px', 'width': '159px', 'float': 'right' } },
        "\xA0"
      )
    ),
    React.createElement(
      "div",
      { className: "col-xs-10" },
      React.createElement(
        "div",
        { className: "row build-cells" },
        React.createElement(
          "div",
          { className: "waterfall-build" },
          React.createElement(
            "div",
            { className: "active-build" },
            TaskTombstones(1)
          )
        )
      )
    )
  );
}

function GridTombstone() {
  return React.createElement(
    "div",
    { className: "waterfall-grid" },
    React.createElement(VariantTombstone, null)
  );
}
//# sourceMappingURL=waterfall.js.map
