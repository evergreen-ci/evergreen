 /*
  ReactJS code for the Waterfall page. Grid calls the Variant class for each distro, and the Variant class renders each build variant for every version that exists. In each build variant we iterate through all the tasks and render them as well. The row of headers is just a placeholder at the moment.
 */

// React doesn't provide own functionality for making http calls
// window.fetch doesn't handle query params
var http = angular.bootstrap().get('$http')

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
  if (!results){
    return null;
  }
  if (!results[2]){
    return '';
  }
  return decodeURIComponent(results[2].replace(/\+/g, " "));
}

function updateURLParams(bvFilter, taskFilter, skip, baseURL, showUpstream) {
  var params = {};
  if (bvFilter && bvFilter != '')
    params["bv_filter"]= bvFilter;
  if (taskFilter && taskFilter != '')
    params["task_filter"]= taskFilter;
  if (skip !== 0) {
    params["skip"] = skip;
  }
  if (showUpstream !== undefined) {
    params["upstream"] = showUpstream
  }

  if (Object.keys(params).length > 0) {
    const paramString = generateURLParameters(params);
    window.history.replaceState({}, '', baseURL + "?" + paramString);
  } else {
    window.history.replaceState({}, '', baseURL);
  }
}

var JIRA_REGEX = /[A-Z]{1,10}-\d{1,6}/ig;

class JiraLink extends React.PureComponent {
  render() {
    var contents

    if (_.isString(this.props.children)) {
      let tokens = this.props.children.split(/\s/);
      let jiraHost = this.props.jiraHost;

      contents = _.map(tokens, function(token, i){
        let hasSpace = i !== (tokens.length - 1);
        let maybeSpace = hasSpace ? ' ': '';
        let capture = '';
        if(capture = token.match(JIRA_REGEX)) {
          let jiraLink = "https://"+jiraHost+"/browse/"+capture;
          return (
             <a href={jiraLink}>{token+maybeSpace}</a>
          );
        } else {
          return token + maybeSpace;
        }
      });
    } else {
      return null;
    }
    return (
      <div>
        {contents}
      </div>
    );
  }
}


// The Root class renders all components on the waterfall page, including the grid view and the filter and new page buttons
// The one exception is the header, which is written in Angular and managed by menu.html
class Root extends React.PureComponent {
  constructor(props){
    super(props);

    const href = window.location.href
    var buildVariantFilter = getParameterByName('bv_filter', href) || ''
    var taskFilter = getParameterByName('task_filter', href) || ''

    var collapsed = localStorage.getItem("collapsed") == "true";
    var showUpstream = localStorage.getItem("show_upstream") == "true";

    this.state = {
      collapsed: collapsed,
      shortenCommitMessage: true,
      buildVariantFilter: buildVariantFilter,
      taskFilter: taskFilter,
      showUpstream: showUpstream,
      data: null
    }
    this.nextSkip = getParameterByName('skip', href) || 0;
    this.baseURL = "/waterfall/" + this.props.project

    // Handle state for a collapsed view, as well as shortened header commit messages
    this.handleCollapseChange = this.handleCollapseChange.bind(this);
    this.onToggleShowUpstream = this.onToggleShowUpstream.bind(this);
    this.handleHeaderLinkClick = this.handleHeaderLinkClick.bind(this);
    this.handleBuildVariantFilter = this.handleBuildVariantFilter.bind(this);
    this.handleTaskFilter = this.handleTaskFilter.bind(this);
    this.loadDataPortion = this.loadDataPortion.bind(this);
    this.loadDataPortion();
    this.loadDataPortion = _.debounce(this.loadDataPortion, 1000)
    this.loadData = this.loadData.bind(this);
  }

  updatePaginationContext(data) {
    // Initialize newer|older buttons
    var versionsOnPage = _.reduce(data.versions, function(m, d) {
      return m + d.authors.length
    }, 0)

    this.currentSkip = data.current_skip
    this.nextSkip = this.currentSkip + versionsOnPage;
    this.prevSkip = this.currentSkip - data.previous_page_count

    if (this.nextSkip >= data.total_versions) {
      this.nextSkip = -1;
    }

    if (this.currentSkip <= 0) {
      this.prevSkip = -1;
    }
  }

  loadData(direction) {
    const skip = direction === -1 ? this.prevSkip : this.nextSkip;
    this.loadDataPortion(undefined, skip);
  }

  loadDataPortion(filter, skip) {
    const params = {
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
    this.setState({data: null});
    http.get(`/rest/v1/waterfall/${this.props.project}`, {params})
      .then(({data}) => {
        this.updatePaginationContext(data);
        this.setState({data, nextSkip: this.nextSkip + data.versions.length});
        updateURLParams(params.bv_filter, this.state.taskFilter, this.currentSkip, this.baseURL);
      });
  }

  handleCollapseChange(collapsed) {
    localStorage.setItem("collapsed", collapsed);
    this.setState({collapsed: collapsed});
  }

  handleBuildVariantFilter(filter) {
    this.loadDataPortion(filter, this.currentSkip);
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

  onToggleShowUpstream() {
    var showUpstream = !this.state.showUpstream;
    this.loadDataPortion(this.state.buildVariantFilter, this.currentSkip);
    localStorage.setItem("show_upstream", showUpstream);
    updateURLParams(this.state.buildVariantFilter, this.state.taskFilter, this.currentSkip, this.baseURL, showUpstream);
    this.setState({showUpstream: showUpstream});
  }

  render() {
    if (this.state.data && this.state.data.rows.length == 0){
      return (
        <div>
          There are no builds for this project.
        </div>
        )
    }
    var collapseInfo = {
      collapsed : this.state.collapsed,
      activeTaskStatuses : ['failed','system-failed'],
    };
    return (
      <div>
        <Toolbar
          collapsed={this.state.collapsed}
          onCheckCollapsed={this.handleCollapseChange}
          baseURL={this.baseURL}
          nextSkip={this.nextSkip}
          prevSkip={this.prevSkip}
          buildVariantFilter={this.state.buildVariantFilter}
          taskFilter={this.state.taskFilter}
          buildVariantFilterFunc={this.handleBuildVariantFilter}
          taskFilterFunc={this.handleTaskFilter}
          isLoggedIn={this.props.user !== null}
          project={this.props.project}
          disabled={false}
          loadData={this.loadData}
          onToggleShowUpstream={this.onToggleShowUpstream}
          showUpstream={this.state.showUpstream}
        />
        <Headers
          shortenCommitMessage={this.state.shortenCommitMessage}
          versions={this.state.data === null ? null : this.state.data.versions}
          onLinkClick={this.handleHeaderLinkClick}
          userTz={this.props.userTz}
          jiraHost={this.props.jiraHost}
        />
        <Grid
          data={this.state.data}
          collapseInfo={collapseInfo}
          project={this.props.project}
          buildVariantFilter={this.state.buildVariantFilter}
          taskFilter={this.state.taskFilter}
        />
      </div>
    )
  }
}


// Toolbar
function Toolbar ({collapsed,
  onCheckCollapsed,
  baseURL,
  nextSkip,
  prevSkip,
  buildVariantFilter,
  taskFilter,
  buildVariantFilterFunc,
  taskFilterFunc,
  isLoggedIn,
  project,
  disabled,
  loadData,
  showUpstream,
  onToggleShowUpstream
}) {

  var Form = ReactBootstrap.Form;
  return (
    <div className="row">
      <div className="col-xs-12">
        <Form inline className="waterfall-toolbar pull-right">
          <CollapseButton collapsed={collapsed} onCheckCollapsed={onCheckCollapsed} disabled={disabled} />
          <FilterBox
            filterFunction={buildVariantFilterFunc}
            placeholder={"Filter variant"}
            currentFilter={buildVariantFilter}
            disabled={disabled}
          />
          <FilterBox
            filterFunction={taskFilterFunc}
            placeholder={"Filter task"}
            currentFilter={taskFilter}
            disabled={collapsed || disabled}
          />
          <PageButtons
            nextSkip={nextSkip}
            prevSkip={prevSkip}
            baseURL={baseURL}
            buildVariantFilter={buildVariantFilter}
            taskFilter={taskFilter}
            disabled={disabled}
            loadData={loadData}
          />
          <GearMenu
            project={project}
            isLoggedIn={isLoggedIn}
            showUpstream={showUpstream}
            onToggleShowUpstream={onToggleShowUpstream}
          />
        </Form>
      </div>
    </div>
  )
};

class PageButtons extends React.PureComponent {
  constructor(props) {
    super(props);
    this.loadNext = () => this.props.loadData(1);
    this.loadPrev = () => this.props.loadData(-1);
  }

  render() {
    const ButtonGroup = ReactBootstrap.ButtonGroup;
    return (
      <span className="waterfall-form-item">
        <ButtonGroup>
          <PageButton disabled={this.props.disabled || this.props.prevSkip < 0} directionIcon="fa-chevron-left" loadData={this.loadPrev} />
          <PageButton disabled={this.props.disabled || this.props.nextSkip < 0} directionIcon="fa-chevron-right" loadData={this.loadNext} />
        </ButtonGroup>
      </span>
    );
  }
}

function PageButton ({directionIcon, disabled, loadData}) {
  var Button = ReactBootstrap.Button;
  var classes = "fa " + directionIcon;
  return (
    <Button onClick={loadData} disabled={disabled}><i className={classes}></i></Button>
  );
}

class FilterBox extends React.PureComponent {
  constructor(props){
    super(props);
    this.applyFilter = this.applyFilter.bind(this);
  }
  applyFilter() {
    this.props.filterFunction(this.refs.searchInput.value)
  }
  render() {
    return <input type="text" ref="searchInput"
                  className="form-control waterfall-form-item"
                  placeholder={this.props.placeholder}
                  value={this.props.currentFilter} onChange={this.applyFilter}
                  disabled={this.props.disabled}/>
  }
}

class CollapseButton extends React.PureComponent {
  constructor(props){
    super(props);
    this.handleChange = this.handleChange.bind(this);
  }
  handleChange(event){
    this.props.onCheckCollapsed(this.refs.collapsedBuilds.checked);
  }
  render() {
    return (
      <span className="semi-muted waterfall-form-item">
        <span id="collapsed-prompt">Show collapsed view</span>
        <input
          className="checkbox waterfall-checkbox"
          type="checkbox"
          checked={this.props.collapsed}
          ref="collapsedBuilds"
          onChange={this.handleChange}
          disabled={this.props.disabled}
        />
      </span>
    )
  }
}

class GearMenu extends React.PureComponent {
  constructor(props) {
    super(props);
    this.addNotification = this.addNotification.bind(this);
    const requesterSubscriberSettings = {text: "Build initiator", key:"requester", type:"select", options:{
        "gitter_request":"Commit",
        "patch_request":"Patch",
        "github_pull_request":"Pull Request",
        "merge_test":"Commit Queue",
        "ad_hoc":"Periodic Build"
      }, default: "gitter_request"}
    this.triggers = [
      {
        trigger: "outcome",
        resource_type: "TASK",
        label: "any task finishes",
        regex_selectors: taskRegexSelectors(),
        extraFields: [requesterSubscriberSettings]
      },
      {
        trigger: "failure",
        resource_type: "TASK",
        label: "any task fails",
        regex_selectors: taskRegexSelectors(),
        extraFields: [
          {text: "Failure type", key:"failure-type", type:"select", options:{"any":"Any","test":"Test","system":"System","setup":"Setup"}, default: "any"},
          requesterSubscriberSettings
        ]
      },
      {
        trigger: "success",
        resource_type: "TASK",
        label: "any task succeeds",
        regex_selectors: taskRegexSelectors(),
        extraFields: [requesterSubscriberSettings]
      },
      {
        trigger: "exceeds-duration",
        resource_type: "TASK",
        label: "the runtime for any task exceeds some duration",
        extraFields: [
          {text: "Task duration (seconds)", key: "task-duration-secs", validator: validateDuration}
        ],
        regex_selectors: taskRegexSelectors()
      },
      {
        trigger: "runtime-change",
        resource_type: "TASK",
        label: "the runtime for a successful task changes by some percentage",
        extraFields: [
          {text: "Percent change", key: "task-percent-change", validator: validatePercentage}
        ],
        regex_selectors: taskRegexSelectors()
      },
      {
        trigger: "outcome",
        resource_type: "BUILD",
        label: "a build-variant in any version finishes",
        regex_selectors: buildRegexSelectors(),
        extraFields: [requesterSubscriberSettings]
      },
      {
        trigger: "failure",
        resource_type: "BUILD",
        label: "a build-variant in any version fails",
        regex_selectors: buildRegexSelectors(),
        extraFields: [requesterSubscriberSettings]
      },
      {
        trigger: "success",
        resource_type: "BUILD",
        label: "a build-variant in any version succeeds",
        regex_selectors: buildRegexSelectors(),
        extraFields: [requesterSubscriberSettings]
      },
      {
        trigger: "outcome",
        resource_type: "VERSION",
        label: "any version finishes",
        extraFields: [requesterSubscriberSettings]
      },
      {
        trigger: "failure",
        resource_type: "VERSION",
        label: "any version fails",
        extraFields: [requesterSubscriberSettings]
      },
      {
        trigger: "success",
        resource_type: "VERSION",
        label: "any version succeeds",
        extraFields: [requesterSubscriberSettings]
      }
    ];
  }

  dialog($mdDialog, $mdToast, notificationService, mciSubscriptionsService) {
    const omitMethods = {
      [SUBSCRIPTION_JIRA_ISSUE]: true,
      [SUBSCRIPTION_EVERGREEN_WEBHOOK]: true
    };

    const self = this;
    const promise = addSubscriber($mdDialog, this.triggers, omitMethods);
    return $mdDialog.show(promise).then(function(data) {
      addProjectSelectors(data, self.project);
      var success = function() {
        return $mdToast.show({
          templateUrl: "/static/partials/subscription_confirmation_toast.html",
          position: "bottom right"
        });
      };
      var failure = function(resp) {
        notificationService.pushNotification('Error saving subscriptions: ' + resp.data.error, 'errorHeader');
      };
      mciSubscriptionsService.post([data], { success: success, error: failure });
    }).catch(function(e) {
      notificationService.pushNotification('Error saving subscriptions: ' + e, 'errorHeader');
    });
  }

  addNotification() {
    const waterfall = angular.module('waterfall', ['ng', 'MCI', 'material.components.toast']);
    waterfall.provider({
      $rootElement: function() {
         this.$get = function() {
           const root = document.getElementById("root");
           return angular.element(root);
        };
      }
    });

    const injector = angular.injector(['waterfall']);
    return injector.invoke(this.dialog, { triggers: this.triggers, project: this.props.project });
  }

  render() {
    if (!this.props.isLoggedIn) {
      return null;
    }
    const ButtonGroup = ReactBootstrap.ButtonGroup;
    const Button = ReactBootstrap.Button;
    const DropdownButton = ReactBootstrap.DropdownButton;
    const MenuItem = ReactBootstrap.MenuItem;

    return (
      <span>
        <DropdownButton className={"fa fa-gear"} pullRight={true} id={"waterfall-gear-menu"}>
          <MenuItem onClick={this.addNotification}>Add Notification</MenuItem>
          <MenuItem onClick={this.props.onToggleShowUpstream}>
            {this.props.showUpstream ? "Hide Upstream Commits" : "Show Upstream Commits"}
          </MenuItem>
        </DropdownButton>
      </span>
    );
  }

};

// Headers

function Headers ({shortenCommitMessage, versions, onLinkClick, userTz, jiraHost}) {
  if (versions === null)  {
    return (<VersionHeaderTombstone />);
  }
  return (
    <div className="row version-header">
      <div className="variant-col col-xs-2 version-header-rolled"></div>
      <div className="col-xs-10">
        <div className="row">
        {
          versions.map(function(version){
            if (version.rolled_up) {
              return(
                <RolledUpVersionHeader
                  key={version.ids[0]}
                  version={version}
                  userTz={userTz}
                  jiraHost={jiraHost}
                />
              );
            }
            // Unrolled up version, no popover
            return (
              <ActiveVersionHeader
                key={version.ids[0]}
                version={version}
                userTz = {userTz}
                shortenCommitMessage={shortenCommitMessage}
                onLinkClick={onLinkClick}
                jiraHost={jiraHost}
              />
            );
          })
        }
        </div>
      </div>
    </div>
  )
}


function VersionHeaderTombstone() {
  return (
    <div className="row version-header">
      <div className="variant-col col-xs-2 version-header-rolled"></div>
      <div className="col-xs-10">
        <div className="row">
          <div className="header-col">
            <div className="version-header-expanded">
              <div className="col-xs-12">
                <div className="row">
                  <div className="waterfall-tombstone" style={{'height': '14px', 'width': '126px'}}>&nbsp;</div>
                </div>
              </div>
              <div className="col-xs-12">
                <div className="row">
                  <div className="waterfall-tombstone" style={{'marginTop': '5px', 'height': '14px', 'width': '78px'}}>&nbsp;</div>
                  <div className="waterfall-tombstone" style={{'marginTop': '5px', 'height': '14px', 'width': '205px'}}>&nbsp;</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function ActiveVersionHeader({shortenCommitMessage, version, onLinkClick, userTz, jiraHost}) {
  var message = version.messages[0];
  var author = version.authors[0];
  var id_link = "/version/" + version.ids[0];
  var commit = version.revisions[0].substring(0,5);
  var tags = gitTagsMessage(version.git_tags[0]);

  if (version.upstream_data) {
    var upstreamData = version.upstream_data[0];
    var upstreamLink = "/" + upstreamData.trigger_type + "/" + upstreamData.trigger_id;
    var upstreamAnchor = <a href={upstreamLink}>{upstreamData.project_name}</a>;
    var upstreamElem = <div className="row"> From {upstreamAnchor} </div>
  }
  var formatted_time = getFormattedTime(version.create_times[0], userTz, 'M/D/YY h:mm A' );
  const maxChars = 44
  var button;
  if (message.length > maxChars) {
    // If we shorten the commit message, only display the first maxChars chars
    if (shortenCommitMessage) {
      message = message.substring(0, maxChars-3) + "...";
    }
    button = (
      <HideHeaderButton onLinkClick={onLinkClick} shortenCommitMessage={shortenCommitMessage} />
    );
  }

  return (
      <div className="header-col">
        <div className="version-header-expanded">
          <div className="col-xs-12">
            <div className="row">
              <a className="githash" href={id_link}>{commit}</a>
              {formatted_time}
            </div>
            {upstreamElem}
          </div>
          <div className="col-xs-12">
            <div className="row">
              <strong>{author}</strong> - <JiraLink jiraHost={jiraHost}>{message}</JiraLink>
              {button}
              <div>{tags}</div>
            </div>
          </div>
        </div>
      </div>
  );
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
      <span onClick={this.onLinkClick}> <a href="#">{textToShow}</a> </span>
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
    <Popover id="popover-positioned-bottom" title="">
      {
        version.ids.map(function(id,i) {
          return <RolledUpVersionSummary
            author={version.authors[i]}
            commit={version.revisions[i]}
            message={version.messages[i]}
            gitTags={version.git_tags[i]}
            versionId={version.ids[i]}
            key={id} userTz={userTz}
            createTime={version.create_times[i]}
            jiraHost={jiraHost} />
        })
      }
    </Popover>
  );

  return (
    <div className="header-col version-header-rolled">
      <OverlayTrigger trigger="click" placement="bottom" overlay={popovers} className="col-xs-2">
          <span className="pointer"> {rolledHeader} </span>
      </OverlayTrigger>
    </div>
  )
};
function RolledUpVersionSummary ({author, commit, message, gitTags, versionId, createTime, userTz, jiraHost}) {
  var formatted_time = getFormattedTime(new Date(createTime), userTz, 'M/D/YY h:mm A' );
  gitTags = gitTagsMessage(gitTags);
  commit = commit.substring(0,10);
  return (
    <div className="rolled-up-version-summary">
      <span className="version-header-time">{formatted_time}</span>
      <br />
      <a href={"/version/" + versionId}>{commit}</a> - <strong>{author}</strong>
      <br />
      <JiraLink jiraHost={jiraHost}>{message}</JiraLink>
      <div>{gitTags}</div>
      <br />
    </div>
  );
}

function TaskTombstones(num) {
  const out = [];
  for (let i = 0; i < num; ++i) {
    out.push((<a className="waterfall-box inactive" />));
  }
  return out;
}

function VariantTombstone() {
  return (
    <div className="row variant-row">
      <div className="col-xs-2 build-variants">
        <div className="waterfall-tombstone" style={{'height': '18px', 'width': '159px', 'float': 'right'}}>&nbsp;</div>
      </div>
      <div className="col-xs-10">
        <div className="row build-cells">
          <div className="waterfall-build">
            <div className="active-build">
              {TaskTombstones(1)}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function GridTombstone() {
  return (
    <div className="waterfall-grid">
      <VariantTombstone />
    </div>
  );
}
