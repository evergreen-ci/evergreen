 /*
  ReactJS code for the Waterfall page. Grid calls the Variant class for each distro, and the Variant class renders each build variant for every version that exists. In each build variant we iterate through all the tasks and render them as well. The row of headers is just a placeholder at the moment.
 */

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
  if (!results){
    return null;
  }
  if (!results[2]){
    return '';
  }
  return decodeURIComponent(results[2].replace(/\+/g, " "));
}

function updateURLParams(bvFilter, taskFilter, skip, baseURL) {
  var params = {};
  if (bvFilter && bvFilter != '')
    params["bv_filter"]= bvFilter;
  if (taskFilter && taskFilter != '')
    params["task_filter"]= taskFilter;
  params["skip"] = skip

  var paramString = generateURLParameters(params);
  window.history.replaceState({}, '', baseURL + "?" + paramString);
}

var JIRA_REGEX = /[A-Z]{1,10}-\d{1,6}/ig;

class JiraLink extends React.Component {
  constructor(props) {
    super(props);
  }

  render() {
    var contents

    if (_.isString(this.props.children)) {
      let tokens = this.props.children.split(/\s/);
      let jiraHost = this.props.jiraHost;

      contents = _.map(tokens, function(token, i){
        let hasSpace = i !== (tokens.length - 1);
        let maybeSpace = hasSpace ? ' ': '';
        let matchingPart = token.match(JIRA_REGEX);
        if(matchingPart && matchingPart.length > 0) {
          let jiraLink = "https://"+jiraHost+"/browse/"+matchingPart;
          return (
             React.createElement("a", {href: jiraLink}, token+maybeSpace)
          );
        } else {
          return token + maybeSpace;
        }
      });
    } else {
      return null;
    }
    return (
      React.createElement("div", null,
        contents
      )
    );
  }
}


// The Root class renders all components on the waterfall page, including the grid view and the filter and new page buttons
// The one exception is the header, which is written in Angular and managed by menu.html
class Root extends React.Component{
  constructor(props){
    super(props);
    // Initialize newer|older buttons
    var versionsOnPage = _.reduce(_.map(window.serverData.versions, function(version){
      return version.authors.length;
    }), function(memo,num){
      return memo + num;
    });

    this.baseURL = "/waterfall/" + this.props.project;
    this.currentSkip = window.serverData.current_skip;
    this.nextSkip = this.currentSkip + versionsOnPage;
    this.prevSkip = this.currentSkip - window.serverData.previous_page_count;

    if (this.nextSkip >= window.serverData.total_versions) {
      this.nextSkip = -1;
    }
    if (this.currentSkip <= 0) {
      this.prevSkip = -1;
    }

    var buildVariantFilter = getParameterByName('bv_filter',window.location.href);
    var taskFilter = getParameterByName('task_filter',window.location.href);

    buildVariantFilter = buildVariantFilter || '';
    taskFilter = taskFilter || '';

    var collapsed = localStorage.getItem("collapsed") == "true";

    this.state = {
      collapsed: collapsed,
      shortenCommitMessage: true,
      buildVariantFilter: buildVariantFilter,
      taskFilter:taskFilter
    };

    // Handle state for a collapsed view, as well as shortened header commit messages
    this.handleCollapseChange = this.handleCollapseChange.bind(this);
    this.handleHeaderLinkClick = this.handleHeaderLinkClick.bind(this);
    this.handleBuildVariantFilter = this.handleBuildVariantFilter.bind(this);
    this.handleTaskFilter = this.handleTaskFilter.bind(this);
  }

  handleCollapseChange(collapsed) {
    localStorage.setItem("collapsed", collapsed);
    this.setState({collapsed: collapsed});
  }
  handleBuildVariantFilter(filter) {
    updateURLParams(filter, this.state.taskFilter, this.currentSkip, this.baseURL);
    this.setState({buildVariantFilter: filter});
  }
  handleTaskFilter(filter) {
    updateURLParams(this.state.buildVariantFilter, filter, this.currentSkip, this.baseURL);
    this.setState({taskFilter: filter});
  }
  handleHeaderLinkClick(shortenMessage) {
    this.setState({shortenCommitMessage: !shortenMessage});
  }
  render() {
    if (this.props.data.rows.length == 0){
      return (
        React.createElement("div", null,
          "There are no builds for this project."
        )
        )
    }
    var collapseInfo = {
      collapsed : this.state.collapsed,
      activeTaskStatuses : ['failed','system-failed'],
    };
    return (
      React.createElement("div", null,
        React.createElement(Toolbar, {
          collapsed: this.state.collapsed,
          onCheck: this.handleCollapseChange,
          baseURL: this.baseURL,
          nextSkip: this.nextSkip,
          prevSkip: this.prevSkip,
          buildVariantFilter: this.state.buildVariantFilter,
          taskFilter: this.state.taskFilter,
          buildVariantFilterFunc: this.handleBuildVariantFilter,
          taskFilterFunc: this.handleTaskFilter}
        ),
        React.createElement(Headers, {
          shortenCommitMessage: this.state.shortenCommitMessage,
          versions: this.props.data.versions,
          onLinkClick: this.handleHeaderLinkClick,
          userTz: this.props.userTz,
          jiraHost: this.props.jiraHost}
        ),
        React.createElement(Grid, {
          data: this.props.data,
          collapseInfo: collapseInfo,
          project: this.props.project,
          buildVariantFilter: this.state.buildVariantFilter,
          taskFilter: this.state.taskFilter}
        )
      )
    )
  }
}


// Toolbar
function Toolbar ({collapsed,
  onCheck,
  baseURL,
  nextSkip,
  prevSkip,
  buildVariantFilter,
  taskFilter,
  buildVariantFilterFunc,
  taskFilterFunc}) {

  var Form = ReactBootstrap.Form;
  return (
    React.createElement("div", {className: "row"},
      React.createElement("div", {className: "col-xs-12"},
        React.createElement(Form, {inline: true, className: "waterfall-toolbar pull-right"},
          React.createElement(CollapseButton, {collapsed: collapsed, onCheck: onCheck}),
          React.createElement(FilterBox, {
            filterFunction: buildVariantFilterFunc,
            placeholder: "Filter variant",
            currentFilter: buildVariantFilter,
            disabled: false}
          ),
          React.createElement(FilterBox, {
            filterFunction: taskFilterFunc,
            placeholder: "Filter task",
            currentFilter: taskFilter,
            disabled: collapsed}
          ),
          React.createElement(PageButtons, {
            nextSkip: nextSkip,
            prevSkip: prevSkip,
            baseURL: baseURL,
            buildVariantFilter: buildVariantFilter,
            taskFilter: taskFilter}
          )
        )
      )
    )
  )
};

function PageButtons ({prevSkip, nextSkip, baseURL, buildVariantFilter, taskFilter}) {
  var ButtonGroup = ReactBootstrap.ButtonGroup;

  var nextURL= "";
  var prevURL= "";

  prevURLParams = {};
  nextURLParams = {};

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
  return (
    React.createElement("span", {className: "waterfall-form-item"},
      React.createElement(ButtonGroup, null,
        React.createElement(PageButton, {pageURL: prevURL, disabled: prevSkip < 0, directionIcon: "fa-chevron-left"}),
        React.createElement(PageButton, {pageURL: nextURL, disabled: nextSkip < 0, directionIcon: "fa-chevron-right"})
      )
    )
  );
}

function PageButton ({pageURL, directionIcon, disabled}) {
  var Button = ReactBootstrap.Button;
  var classes = "fa " + directionIcon;
  return (
    React.createElement(Button, {href: pageURL, disabled: disabled}, React.createElement("i", {className: classes}))
  );
}

class FilterBox extends React.Component {
  constructor(props){
    super(props);
    this.applyFilter = this.applyFilter.bind(this);
  }
  applyFilter() {
    this.props.filterFunction(this.refs.searchInput.value)
  }
  render() {
    return React.createElement("input", {type: "text", ref: "searchInput",
                  className: "form-control waterfall-form-item",
                  placeholder: this.props.placeholder,
                  value: this.props.currentFilter, onChange: this.applyFilter,
                  disabled: this.props.disabled})
  }
}

class CollapseButton extends React.Component{
  constructor(props){
    super(props);
    this.handleChange = this.handleChange.bind(this);
  }
  handleChange(event){
    this.props.onCheck(this.refs.collapsedBuilds.checked);
  }
  render() {
    return (
      React.createElement("span", {className: "semi-muted waterfall-form-item"},
        React.createElement("span", {id: "collapsed-prompt"}, "Show collapsed view"),
        React.createElement("input", {
          className: "checkbox waterfall-checkbox",
          type: "checkbox",
          checked: this.props.collapsed,
          ref: "collapsedBuilds",
          onChange: this.handleChange}
        )
      )
    )
  }
}

// Headers

function Headers ({shortenCommitMessage, versions, onLinkClick, userTz, jiraHost}) {
  return (
    React.createElement("div", {className: "row version-header"},
      React.createElement("div", {className: "variant-col col-xs-2 version-header-rolled"}),
      React.createElement("div", {className: "col-xs-10"},
        React.createElement("div", {className: "row"},

          versions.map(function(version){
            if (version.rolled_up) {
              return(
                React.createElement(RolledUpVersionHeader, {
                  key: version.ids[0],
                  version: version,
                  userTz: userTz,
                  jiraHost: jiraHost}
                )
              );
            }
            // Unrolled up version, no popover
            return (
              React.createElement(ActiveVersionHeader, {
                key: version.ids[0],
                version: version,
                userTz: userTz,
                shortenCommitMessage: shortenCommitMessage,
                onLinkClick: onLinkClick,
                jiraHost: jiraHost}
              )
            );
          })

        )
      )
    )
  )
}


function ActiveVersionHeader({shortenCommitMessage, version, onLinkClick, userTz, jiraHost}) {
  var message = version.messages[0];
  var author = version.authors[0];
  var id_link = "/version/" + version.ids[0];
  var commit = version.revisions[0].substring(0,5);
  var message = version.messages[0];
  var formatted_time = getFormattedTime(version.create_times[0], userTz, 'M/D/YY h:mm A' );
  const maxChars = 44
  var button;
  if (message.length > maxChars) {
    // If we shorten the commit message, only display the first maxChars chars
    if (shortenCommitMessage) {
      message = message.substring(0, maxChars-3) + "...";
    }
    button = (
      React.createElement(HideHeaderButton, {onLinkClick: onLinkClick, shortenCommitMessage: shortenCommitMessage})
    );
  }

  return (
      React.createElement("div", {className: "header-col"},
        React.createElement("div", {className: "version-header-expanded"},
          React.createElement("div", {className: "col-xs-12"},
            React.createElement("div", {className: "row"},
              React.createElement("a", {className: "githash", href: id_link}, commit),
              formatted_time
            )
          ),
          React.createElement("div", {className: "col-xs-12"},
            React.createElement("div", {className: "row"},
              React.createElement("strong", null, author), " - ", React.createElement(JiraLink, {jiraHost: jiraHost}, message),
              button
            )
          )
        )
      )
  )
};

class HideHeaderButton extends React.Component{
  constructor(props){
    super(props);
    this.onLinkClick = this.onLinkClick.bind(this);
  }
  onLinkClick(event){
    this.props.onLinkClick(this.props.shortenCommitMessage);
  }
  render() {
    var textToShow = this.props.shortenCommitMessage ? "more" : "less";
    return (
      React.createElement("span", {onClick: this.onLinkClick}, " ", React.createElement("a", {href: "#"}, textToShow), " ")
    )
  }
}

function RolledUpVersionHeader({version, userTz, jiraHost}){
  var Popover = ReactBootstrap.Popover;
  var OverlayTrigger = ReactBootstrap.OverlayTrigger;
  var Button = ReactBootstrap.Button;

  var versionStr = (version.messages.length > 1) ? "versions" : "version";
  var rolledHeader = version.messages.length + " inactive " + versionStr;

  var popovers = (
    React.createElement(Popover, {id: "popover-positioned-bottom", title: ""},

        version.ids.map(function(id,i) {
          return React.createElement(RolledUpVersionSummary, {
            author: version.authors[i],
            commit: version.revisions[i],
            message: version.messages[i],
            versionId: version.ids[i],
            key: id, userTz: userTz,
            createTime: version.create_times[i],
            jiraHost: jiraHost})
        })

    )
  );

  return (
    React.createElement("div", {className: "header-col version-header-rolled"},
      React.createElement(OverlayTrigger, {trigger: "click", placement: "bottom", overlay: popovers, className: "col-xs-2"},
          React.createElement("span", {className: "pointer"}, " ", rolledHeader, " ")
      )
    )
  )
};
function RolledUpVersionSummary ({author, commit, message, versionId, createTime, userTz, jiraHost}) {
  var formatted_time = getFormattedTime(new Date(createTime), userTz, 'M/D/YY h:mm A' );
  commit =  commit.substring(0,10);

  return (
    React.createElement("div", {className: "rolled-up-version-summary"},
      React.createElement("span", {className: "version-header-time"}, formatted_time),
      React.createElement("br", null),
      React.createElement("a", {href: "/version/" + versionId}, commit), " - ", React.createElement("strong", null, author),
      React.createElement("br", null),
      React.createElement(JiraLink, {jiraHost: jiraHost}, message),
      React.createElement("br", null)
    )
  );
}
