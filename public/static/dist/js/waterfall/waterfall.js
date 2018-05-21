"use strict";

var _createClass = function () { function defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } } return function (Constructor, protoProps, staticProps) { if (protoProps) defineProperties(Constructor.prototype, protoProps); if (staticProps) defineProperties(Constructor, staticProps); return Constructor; }; }();

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

function updateURLParams(bvFilter, taskFilter, skip, baseURL) {
  var params = {};
  if (bvFilter && bvFilter != '') params["bv_filter"] = bvFilter;
  if (taskFilter && taskFilter != '') params["task_filter"] = taskFilter;
  params["skip"] = skip;

  var paramString = generateURLParameters(params);
  window.history.replaceState({}, '', baseURL + "?" + paramString);
}

var JIRA_REGEX = /[A-Z]{1,10}-\d{1,6}/ig;

var JiraLink = function (_React$Component) {
  _inherits(JiraLink, _React$Component);

  function JiraLink(props) {
    _classCallCheck(this, JiraLink);

    return _possibleConstructorReturn(this, (JiraLink.__proto__ || Object.getPrototypeOf(JiraLink)).call(this, props));
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
          if (token.match(JIRA_REGEX)) {
            var jiraLink = "https://" + jiraHost + "/browse/" + token;
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
}(React.Component);

// The Root class renders all components on the waterfall page, including the grid view and the filter and new page buttons
// The one exception is the header, which is written in Angular and managed by menu.html


var Root = function (_React$Component2) {
  _inherits(Root, _React$Component2);

  function Root(props) {
    _classCallCheck(this, Root);

    var _this2 = _possibleConstructorReturn(this, (Root.__proto__ || Object.getPrototypeOf(Root)).call(this, props));

    _this2.updatePaginationContext(window.serverData);

    var href = window.location.href;
    var buildVariantFilter = getParameterByName('bv_filter', href) || '';
    var taskFilter = getParameterByName('task_filter', href) || '';

    var collapsed = localStorage.getItem("collapsed") == "true";

    _this2.state = {
      collapsed: collapsed,
      shortenCommitMessage: true,
      buildVariantFilter: buildVariantFilter,
      taskFilter: taskFilter,
      data: _this2.props.data

      // Handle state for a collapsed view, as well as shortened header commit messages
    };_this2.handleCollapseChange = _this2.handleCollapseChange.bind(_this2);
    _this2.handleHeaderLinkClick = _this2.handleHeaderLinkClick.bind(_this2);
    _this2.handleBuildVariantFilter = _this2.handleBuildVariantFilter.bind(_this2);
    _this2.handleTaskFilter = _this2.handleTaskFilter.bind(_this2);
    _this2.loadDataPortion = _.debounce(_this2.loadDataPortion, 100);
    return _this2;
  }

  _createClass(Root, [{
    key: "updatePaginationContext",
    value: function updatePaginationContext(data) {
      console.log(data);
      // Initialize newer|older buttons
      var versionsOnPage = _.reduce(data.versions, function (m, d) {
        return m + d.authors.length;
      }, 0);

      this.baseURL = "/waterfall/" + this.props.project;
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
    key: "loadDataPortion",
    value: function loadDataPortion(filter) {
      var _this3 = this;

      var params = filter ? { bv_filter: filter } : {};
      http.get("/rest/v2/waterfall/" + this.props.project, { params: params }).then(function (_ref) {
        var data = _ref.data;

        _this3.updatePaginationContext(data);
        _this3.setState({ data: data });
        updateURLParams(filter, _this3.state.taskFilter, _this3.currentSkip, _this3.baseURL);
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
      this.loadDataPortion(filter);
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
    key: "render",
    value: function render() {
      if (this.state.data.rows.length == 0) {
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
          onCheck: this.handleCollapseChange,
          baseURL: this.baseURL,
          nextSkip: this.nextSkip,
          prevSkip: this.prevSkip,
          buildVariantFilter: this.state.buildVariantFilter,
          taskFilter: this.state.taskFilter,
          buildVariantFilterFunc: this.handleBuildVariantFilter,
          taskFilterFunc: this.handleTaskFilter
        }),
        React.createElement(Headers, {
          shortenCommitMessage: this.state.shortenCommitMessage,
          versions: this.state.data.versions,
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
}(React.Component);

// Toolbar


function Toolbar(_ref2) {
  var collapsed = _ref2.collapsed,
      onCheck = _ref2.onCheck,
      baseURL = _ref2.baseURL,
      nextSkip = _ref2.nextSkip,
      prevSkip = _ref2.prevSkip,
      buildVariantFilter = _ref2.buildVariantFilter,
      taskFilter = _ref2.taskFilter,
      buildVariantFilterFunc = _ref2.buildVariantFilterFunc,
      taskFilterFunc = _ref2.taskFilterFunc;


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
        React.createElement(CollapseButton, { collapsed: collapsed, onCheck: onCheck }),
        React.createElement(FilterBox, {
          filterFunction: buildVariantFilterFunc,
          placeholder: "Filter variant",
          currentFilter: buildVariantFilter,
          disabled: false
        }),
        React.createElement(FilterBox, {
          filterFunction: taskFilterFunc,
          placeholder: "Filter task",
          currentFilter: taskFilter,
          disabled: collapsed
        }),
        React.createElement(PageButtons, {
          nextSkip: nextSkip,
          prevSkip: prevSkip,
          baseURL: baseURL,
          buildVariantFilter: buildVariantFilter,
          taskFilter: taskFilter
        })
      )
    )
  );
};

function PageButtons(_ref3) {
  var prevSkip = _ref3.prevSkip,
      nextSkip = _ref3.nextSkip,
      baseURL = _ref3.baseURL,
      buildVariantFilter = _ref3.buildVariantFilter,
      taskFilter = _ref3.taskFilter;

  var ButtonGroup = ReactBootstrap.ButtonGroup;

  var nextURL = "";
  var prevURL = "";

  var prevURLParams = {};
  var nextURLParams = {};

  nextURLParams["skip"] = nextSkip;
  prevURLParams["skip"] = prevSkip;
  if (buildVariantFilter && buildVariantFilter != '') {
    nextURLParams["bv_filter"] = buildVariantFilter;
    prevURLParams["bv_filter"] = buildVariantFilter;
  }
  if (taskFilter && taskFilter != '') {
    nextURLParams["task_filter"] = taskFilter;
    prevURLParams["task_filter"] = taskFilter;
  }
  nextURL = "?" + generateURLParameters(nextURLParams);
  prevURL = "?" + generateURLParameters(prevURLParams);
  return React.createElement(
    "span",
    { className: "waterfall-form-item" },
    React.createElement(
      ButtonGroup,
      null,
      React.createElement(PageButton, { pageURL: prevURL, disabled: prevSkip < 0, directionIcon: "fa-chevron-left" }),
      React.createElement(PageButton, { pageURL: nextURL, disabled: nextSkip < 0, directionIcon: "fa-chevron-right" })
    )
  );
}

function PageButton(_ref4) {
  var pageURL = _ref4.pageURL,
      directionIcon = _ref4.directionIcon,
      disabled = _ref4.disabled;

  var Button = ReactBootstrap.Button;
  var classes = "fa " + directionIcon;
  return React.createElement(
    Button,
    { href: pageURL, disabled: disabled },
    React.createElement("i", { className: classes })
  );
}

var FilterBox = function (_React$Component3) {
  _inherits(FilterBox, _React$Component3);

  function FilterBox(props) {
    _classCallCheck(this, FilterBox);

    var _this4 = _possibleConstructorReturn(this, (FilterBox.__proto__ || Object.getPrototypeOf(FilterBox)).call(this, props));

    _this4.applyFilter = _this4.applyFilter.bind(_this4);
    return _this4;
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
}(React.Component);

var CollapseButton = function (_React$Component4) {
  _inherits(CollapseButton, _React$Component4);

  function CollapseButton(props) {
    _classCallCheck(this, CollapseButton);

    var _this5 = _possibleConstructorReturn(this, (CollapseButton.__proto__ || Object.getPrototypeOf(CollapseButton)).call(this, props));

    _this5.handleChange = _this5.handleChange.bind(_this5);
    return _this5;
  }

  _createClass(CollapseButton, [{
    key: "handleChange",
    value: function handleChange(event) {
      this.props.onCheck(this.refs.collapsedBuilds.checked);
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
          onChange: this.handleChange
        })
      );
    }
  }]);

  return CollapseButton;
}(React.Component);

// Headers

function Headers(_ref5) {
  var shortenCommitMessage = _ref5.shortenCommitMessage,
      versions = _ref5.versions,
      onLinkClick = _ref5.onLinkClick,
      userTz = _ref5.userTz,
      jiraHost = _ref5.jiraHost;

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

function ActiveVersionHeader(_ref6) {
  var shortenCommitMessage = _ref6.shortenCommitMessage,
      version = _ref6.version,
      onLinkClick = _ref6.onLinkClick,
      userTz = _ref6.userTz,
      jiraHost = _ref6.jiraHost;

  var message = version.messages[0];
  var author = version.authors[0];
  var id_link = "/version/" + version.ids[0];
  var commit = version.revisions[0].substring(0, 5);
  var message = version.messages[0];
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
        )
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
          button
        )
      )
    )
  );
};

var HideHeaderButton = function (_React$Component5) {
  _inherits(HideHeaderButton, _React$Component5);

  function HideHeaderButton(props) {
    _classCallCheck(this, HideHeaderButton);

    var _this6 = _possibleConstructorReturn(this, (HideHeaderButton.__proto__ || Object.getPrototypeOf(HideHeaderButton)).call(this, props));

    _this6.onLinkClick = _this6.onLinkClick.bind(_this6);
    return _this6;
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

function RolledUpVersionHeader(_ref7) {
  var version = _ref7.version,
      userTz = _ref7.userTz,
      jiraHost = _ref7.jiraHost;

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
function RolledUpVersionSummary(_ref8) {
  var author = _ref8.author,
      commit = _ref8.commit,
      message = _ref8.message,
      versionId = _ref8.versionId,
      createTime = _ref8.createTime,
      userTz = _ref8.userTz,
      jiraHost = _ref8.jiraHost;

  var formatted_time = getFormattedTime(new Date(createTime), userTz, 'M/D/YY h:mm A');
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
    React.createElement("br", null)
  );
}
//# sourceMappingURL=waterfall.js.map
