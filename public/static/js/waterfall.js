  /*
  ReactJS code for the Waterfall page. Grid calls the Variant class for each distro, and the Variant class renders each build variant for every version that exists. In each build variant we iterate through all the tasks and render them as well. The row of headers is just a placeholder at the moment.
  */

// Given a version id, build id, and server data, returns the build associated with it 
function getBuildByIds(versionId, buildId, data) {
  if (data.versions[versionId].builds != null) {
    return data.versions[versionId].builds[buildId];
  }
  return null;
}

// Preprocess the data given by the server 
// Sort the array of builds for each version, as well as the array of build variants
function preProcessData(data) {
  // Comparison function used to sort the builds for each version
  function comp(a, b) {
      if (a.build_variant.display_name > b.build_variant.display_name) return 1;
      if (a.build_variant.display_name < b.build_variant.display_name) return -1;
      return 0;
    }

  // Iterate over each version and sort the list of builds for unrolled up versions 
  // Keep track of an index for an unrolled up version as well

  _.each(data.versions, function(version, i) {
    if (!version.rolled_up) {
      data.unrolledVersionIndex = i;
      data.versions[i].builds = version.builds.sort(comp);
    }
  });

  //Sort the build variants that Grid uses to show the build column on the left-hand side
  data.build_variants = data.build_variants.sort();
}

// Returns string from datetime object in "5/7/96 1:15 AM" format
// Used to display version headers
function getFormattedTime(datetimeObj) {
  var formatted_time = datetimeObj.toLocaleDateString('en-US', {
    month : 'numeric',
    day : 'numeric',
    year : '2-digit',
    hour : '2-digit',
    minute : '2-digit'
  }).replace(",","");

  return formatted_time;
}

preProcessData(window.serverData);

// The Root class renders all components on the waterfall page, including the grid view and the filter and new page buttons
// The one exception is the header, which is written in Angular and managed by menu.html
class Root extends React.Component{
  constructor(props){
    super(props);
    this.state = {collapsed: false};
    this.handleCollapseChange = this.handleCollapseChange.bind(this);
  }
  handleCollapseChange(collapsed) {
    this.setState({collapsed: collapsed});
  }
  render() {
    var toolbarData = {collapsed : this.state.collapsed,
                       onCheck : this.handleCollapseChange};
    var gridData = {data : this.props.data,
                    collapsed : this.state.collapsed};
    return (
      React.createElement("div", null, 
        React.createElement(Toolbar, {data: this.props.data, collapsed: this.state.collapsed, onCheck: this.handleCollapseChange, toolbarData: toolbarData, stuff: "hellolol", things: "nope"}), 
        React.createElement(Headers, {versions: this.props.data.versions}), 
        React.createElement(Grid, {gridData: gridData})
      )
    )
  }
}

/*** START OF WATERFALL TOOLBAR ***/

const Toolbar = ({toolbarData}) => {
  return (
    React.createElement(CollapseButton, {collapsed: toolbarData.collapsed, onCheck: toolbarData.onCheck}) 
  )
};

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
      React.createElement("label", {style: {display:"inline-block"}}, 
        React.createElement("span", {style: {fontWeight:"normal"}}, "Show Collapsed View "), React.createElement("input", {style: {display:"inline"}, 
          className: "checkbox", 
          type: "checkbox", 
          checked: this.props.collapsed, 
          ref: "collapsedBuilds", 
          onChange: this.handleChange})
      )
    )
  }
}

/*** START OF WATERFALL HEADERS ***/

const Headers = ({versions}) => {
  return (
  React.createElement("div", {className: "row version-header"}, 
    React.createElement("div", {className: "variant-col col-xs-2 version-header-full text-right"}, 
      "Variant"
    ), 
    
      versions.map((version,i) => {
        if (version.rolled_up) {
          return React.createElement(RolledUpVersionHeader, {key: version.ids[0], version: version})
        }
        // Unrolled up version, no popover
        return React.createElement(ActiveVersionHeader, {key: version.ids[0], version: version});
      }), 
    
    React.createElement("br", null)
  )
  )
}

const ActiveVersionHeader = ({version}) => {
  var currVersion = version;
  var message = currVersion.messages[0];

  var author = currVersion.authors[0];
  var id_link = "/version/" + currVersion.ids[0];
  var commit = currVersion.revisions[0].substring(0,5);
  var message = currVersion.messages[0]; 
  var shortened_message = currVersion.messages[0].substring(0,35);

  var formatted_time = getFormattedTime(new Date(currVersion.create_times[0]));

  return (
      React.createElement("div", {className: "col-xs-2"}, 
        React.createElement("div", {className: "version-header-expanded"}, 
          React.createElement("div", null, 
            React.createElement("span", {className: "btn btn-default btn-hash history-item-revision"}, 
              React.createElement("a", {href: id_link}, commit)
            ), 
            formatted_time
          ), 
          author, " - ", shortened_message
        )
      )
  )
};

const RolledUpVersionHeader = ({version}) => {
  var Popover = ReactBootstrap.Popover;
  var OverlayTrigger = ReactBootstrap.OverlayTrigger;
  var Button = ReactBootstrap.Button;

  var currVersion = version;
  
  var versiontitle = currVersion.messages.length > 1 ? "versions" : "version";
  var rolled_header = currVersion.messages.length + " inactive " + versiontitle; 
 
  var versionData = {};
  versionData.version = currVersion; 
  const popovers = (
    React.createElement(Popover, {id: "popover-positioned-bottom", title: ""}, 
      
        currVersion.ids.map(function(x,i) {
          versionData.index = i;
          return React.createElement(RolledUpVersionSummary, {versionData: versionData, currentVersion: currVersion, currentIndex: i})
        })
      
    )
  );

  return (
    React.createElement("div", {className: "col-xs-2"}, 
      React.createElement(OverlayTrigger, {trigger: "click", placement: "bottom", overlay: popovers, className: "col-xs-2"}, 
        React.createElement(Button, {className: "rolled-up-button"}, 
          React.createElement("a", {href: "#"}, rolled_header)
        )
      )
    )
  )
};

const RolledUpVersionSummary = ({versionData}) => {
  var version = versionData.version;
  var i = versionData.index;

  var formatted_time = getFormattedTime(new Date(version.create_times[i]));
  var author = version.authors[i];
  var commit =  version.revisions[i].substring(0,10);
  var message = version.messages[i];
    
  return (
    React.createElement("div", {className: "rolled-up-version-summary"}, 
      React.createElement("span", {className: "version-header-time"}, formatted_time), 
      React.createElement("br", null), 
      React.createElement("a", {href: "/version/" + version.ids[i]}, commit), " - ", React.createElement("strong", null, author), 
      React.createElement("br", null), 
      message, 
      React.createElement("br", null)
    )
  )
};

/*** START OF WATERFALL GRID ***/

// The main class that binds to the root div. This contains all the distros, builds, and tasks

const Grid = ({gridData}) => {
  var data = gridData.data;
  var collapsed = gridData.collapsed;
  return (
    React.createElement("div", {className: "waterfall-grid"}, 
      
        data.build_variants.map((x, i) => {
          return React.createElement(Variant, {key: x, data: data, variantIndex: i, variantDisplayName: x, collapsed: collapsed});
        })
      
    ) 
  )
};

// The class for each "row" of the waterfall page. Includes the build variant link, as well as the five columns
// of versions.
class Variant extends React.Component{
  render() {
    var data = this.props.data;
    var variantIndex = this.props.variantIndex;
    var variantId = getBuildByIds(data.unrolledVersionIndex, variantIndex, data).build_variant.id;
    
    return (
      React.createElement("div", {className: "row variant-row"}, 

        /* column of build names */
        React.createElement("div", {className: "col-xs-2 build-variant-name distro-col"}, 
          React.createElement("a", {href: "/build_variant/" + project + "/" + variantId}, 
            this.props.variantDisplayName
          )
        ), 

        /* 5 columns of versions */
        React.createElement("div", {className: "col-xs-10"}, 
          React.createElement("div", {className: "row build-cols"}, 
            
              data.versions.map((x,i) => {
                var buildData = {};
                buildData.collapsed = this.props.collapsed;
                buildData.build = getBuildByIds(i, variantIndex, data);
                buildData.currentVersion = data.versions[i];

                return React.createElement(Build, {key: x.ids[0], build: buildData})
              })
            
          )
        )

      )
    )
  }
}

// Each Build class is one group of tasks for an version + build variant intersection
// We case on whether or not a build is active or not, and return either an ActiveBuild or InactiveBuild respectively
const Build = ({build}) => {
  
  if (build.currentVersion.rolled_up) {
    return React.createElement(InactiveBuild, {className: "build"});
  }
 
  var buildData = {};
  buildData.build = build.build;
  
  if (build.collapsed) {
    buildData.filter = ['failed','sytem-failed']; // Can be modified to show combinations of tasks by statuses      
    return (
      React.createElement("div", {className: "build"}, 
        React.createElement(ActiveBuild, {buildData: buildData}), 
        React.createElement(CollapsedBuild, {buildData: buildData})
      )
    )
  } 
  
  //We have an active, uncollapsed build 
  return (
    React.createElement("div", {className: "build"}, 
      React.createElement(ActiveBuild, {buildData: buildData})
    )
  )
}

// At least one task in the version is non-inactive, so we display all build tasks with their appropiate colors signifying their status
const ActiveBuild = ({buildData}) => {  
  var tasks = buildData.build.tasks;
  var validTasks = buildData.filter;

  // If our filter is defined, we filter our list of tasks to only display certain types
  // Currently we only filter on status, but it would be easy to filter on other task attributes
  if (validTasks != null) {
    tasks = _.filter(tasks, ((task) => { 
      for (var i = 0; i < validTasks.length; i++) {
        if (validTasks[i] === task.status) return true;
      }
      return false;
    }));
  }

  return (
    React.createElement("div", {className: "active-build"}, 
      
        tasks.map((task) => {
          return React.createElement(Task, {task: task})
        })
      
    )
  )
}

// All tasks are inactive, so we display the words "inactive build"
const InactiveBuild = ({}) => {
    return (
      React.createElement("div", {className: "inactive-build"}, " inactive build ")
    )
}

// A Task contains the information for a single task for a build, including the link to its page, and a tooltip
const Task = ({task}) => {
  var status = task.status;
  var tooltipContent = task.display_name + " - " + status;

  return (
    React.createElement("div", {className: "waterfall-box"}, 
      React.createElement("a", {href: "/task/" + task.id, className: "task-result " + status})
    )
  )
}

// A CollapsedBuild contains a set of PartialProgressBars, which in turn make up a full progress bar
// We iterate over the 5 different main types of task statuses, each of which have a different color association
const CollapsedBuild = ({buildData}) => {
  var build = buildData.build;
  var taskStats = build.waterfallTaskStatusCount;
 
  var taskTypes = [ 
                    ["success"      , taskStats.succeeded], 
                    ["dispatched"   , taskStats.started], 
                    ["system-failed", taskStats.timed_out],
                    ["undispatched" , taskStats.undispatched], 
                    ["inactive"     , taskStats.inactive]
                  ];

  // Remove all task summaries that have 0 tasks
  taskTypes = _.filter(taskTypes,((x => { 
    return x[1] > 0;
  })));
  
  return (
    React.createElement("div", {className: "collapsed-bar"}, 
      
        taskTypes.map((x) => {
          var taskSummaryData = {};
          taskSummaryData.status = x[0];
          taskSummaryData.taskNum = x[1];
          return React.createElement(TaskSummary, {key: x[0], taskSummaryData: taskSummaryData, total: build.tasks.length, status: x[0], taskNum: x[1]});
        }) 
      
    )
  )
}

// A TaskSummary is the class for one rolled up task type
// A CollapsedBuild is comprised of an  array of contiguous TaskSummaries below individual failing tasks 
const TaskSummary = ({taskSummaryData}) => {
  return (
    React.createElement("div", {className: taskSummaryData.status + " task-summary"}, 
      "+", taskSummaryData.taskNum
    )
  )
}
