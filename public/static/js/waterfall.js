  /*
  ReactJS code for the Waterfall page. Grid calls the Variant class for each distro, and the Variant class renders each build variant for every version that exists. In each build variant we iterate through all the tasks and render them as well. The row of headers is just a placeholder at the moment.
  */



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


// The Root class renders all components on the waterfall page, including the grid view and the filter and new page buttons
// The one exception is the header, which is written in Angular and managed by menu.html
class Root extends React.Component{
  constructor(props){
    super(props);
    this.state = {collapsed : false,
                  hidden    : true};
    this.handleCollapseChange = this.handleCollapseChange.bind(this);
    this.handleHideChange = this.handleHideChange.bind(this);
  }
  handleCollapseChange(collapsed) {
    this.setState({collapsed: collapsed});
  }
  handleHideChange(hidden) {
    var opposite = !hidden;
    this.setState({hidden: opposite});
  }
  render() {
    return (
      React.createElement("div", null, 
        React.createElement(Toolbar, {data: this.props.data, collapsed: this.state.collapsed, onCheck: this.handleCollapseChange, toolbarData: toolbarData}), 
        React.createElement(Headers, {versions: this.props.data.versions, hidden: this.state.hidden, onHide: this.handleHideChange}), 
        React.createElement(Grid, {data: this.props.data, collapsed: this.state.collapsed, project: this.props.project})
      )
    )
  }
}

/*** START OF WATERFALL TOOLBAR ***/

const Toolbar = ({collapsed, onCheck}) => {
  return (
    React.createElement(CollapseButton, {collapsed: collapsed, onCheck: onCheck}) 
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


function Headers ({versions, hidden, onHide}) {
  var versionList = _.sortBy(_.values(versions), 'revision_order').reverse();
  return (
  React.createElement("div", {className: "row version-header"}, 
    React.createElement("div", {className: "variant-col col-xs-2 version-header-full text-right"}, 
      "Variant"
    ), 
    
      _.map(versionList, function(version){
        if (version.rolled_up) {
          return React.createElement(RolledUpVersionHeader, {key: version.ids[0], version: version})
        }
        // Unrolled up version, no popover
        return React.createElement(ActiveVersionHeader, {key: version.ids[0], version: version, hidden: hidden, onHide: onHide});

      }), 
    
    React.createElement("br", null)
  )
  )
}

function ActiveVersionHeader({version, hidden, onHide}) {
  var message = version.messages[0];
  var author = version.authors[0];
  var id_link = "/version/" + version.ids[0];
  var commit = version.revisions[0].substring(0,5);
  var message = version.messages[0]; 
  var formatted_time = getFormattedTime(new Date(version.create_times[0]));
  
  //If we hide the full commit message, only take the first 35 chars
  if (hidden) message = message.substring(0,35) + "...";

  return (
      React.createElement("div", {className: "col-xs-2"}, 
        React.createElement("div", {className: "version-header-expanded"}, 
          React.createElement("div", null, 
            React.createElement("span", {className: "btn btn-default btn-hash history-item-revision"}, 
              React.createElement("a", {href: id_link}, commit)
            ), 
            formatted_time
          ), 
          author, " - ", message, 
          React.createElement(HideHeaderButton, {onHide: onHide, hidden: hidden})
        )
      )
  )
};

class HideHeaderButton extends React.Component{
  constructor(props){
    super(props);
    this.handleHide = this.handleHide.bind(this);
  }
  handleHide(event){
    this.props.onHide(this.props.hidden);
  }
  render() {
    var textToShow = this.props.hidden ? "more" : "less";
    return (
      React.createElement("span", {onClick: this.handleHide}, " ", React.createElement("a", {href: "#"}, textToShow), " ")
    )
  }
}

function RolledUpVersionHeader({version}){
  var Popover = ReactBootstrap.Popover;
  var OverlayTrigger = ReactBootstrap.OverlayTrigger;
  var Button = ReactBootstrap.Button;
  
  var versionStr = (version.messages.length > 1) ? "versions" : "version";
  var rolledHeader = version.messages.length + " inactive " + versionStr; 
 
  const popovers = (
    React.createElement(Popover, {id: "popover-positioned-bottom", title: ""}, 
      
        version.ids.map(function(id,i) {
          return React.createElement(RolledUpVersionSummary, {version: version, key: id, i: i})
        })
      
    )
  );

  return (
    React.createElement("div", {className: "col-xs-2"}, 
      React.createElement(OverlayTrigger, {trigger: "click", placement: "bottom", overlay: popovers, className: "col-xs-2"}, 
        React.createElement(Button, {className: "rolled-up-button"}, 
          React.createElement("a", {href: "#"}, rolledHeader)
        )
      )
    )
  )
};

function RolledUpVersionSummary ({version, i}) {
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
function Grid ({data, project, collapsed}) {
  return (
    React.createElement("div", {className: "waterfall-grid"}, 
      
        data.rows.map(function(row){
          return React.createElement(Variant, {row: row, project: project, collapsed: collapsed, versions: data.versions});
        })
      
    ) 
  )
};

// The class for each "row" of the waterfall page. Includes the build variant link, as well as the five columns
// of versions.
function Variant({row, versions, project, collapsed}) {
      return (
      React.createElement("div", {className: "row variant-row"}, 
        React.createElement("div", {className: "col-xs-2 build-variant-name distro-col"}, 
        React.createElement("a", {href: "/build_variant/" + project + "/" + row.build_variant.id}, 
            row.build_variant.display_name
          )
        ), 
        React.createElement("div", {className: "col-xs-10"}, 
          React.createElement("div", {className: "row build-cols"}, 
            
              row.versions.map((versionId,i) => {
                return React.createElement(Build, {key: versionId, build: row.builds[versionId], version: versions[versionId], collapsed: collapsed})
              })
            
          )
        )
      )
    )
}


// Each Build class is one group of tasks for an version + build variant intersection
// We case on whether or not a build is active or not, and return either an ActiveBuild or InactiveBuild respectively
function Build({build, collapsed, version}){
  // inactive build
  if (version.rolled_up) {
    return React.createElement(InactiveBuild, {className: "build"});
  }
  // collapsed active build
  if (collapsed) {
    var validTasks = ['failed','system-failed']; // Can be modified to show combinations of tasks by statuses      
    return (
      React.createElement("div", {className: "build"}, 
        React.createElement(ActiveBuild, {build: build, validTasks: validTasks}), 
        React.createElement(CollapsedBuild, {build: build, validTasks: validTasks})
      )
    )
  } 
  // uncollapsed active build
  return (
    React.createElement("div", {className: "build"}, 
      React.createElement(ActiveBuild, {build: build})
    )
  )
}

// At least one task in the version is non-inactive, so we display all build tasks with their appropiate colors signifying their status
function ActiveBuild({build, validTasks}){  
  var tasks = build.tasks;

  // If our filter is defined, we filter our list of tasks to only display a given status 
  if (validTasks != null) {
    tasks = _.filter(tasks, function(task) { 
      return _.contains(validTasks, task.status);
    });
  }

  return (
    React.createElement("div", {className: "active-build"}, 
      
        tasks.map(function(task){
          return React.createElement(Task, {task: task})
        })
      
    )
  )
}

// All tasks are inactive, so we display the words "inactive build"
function InactiveBuild ({}){
    return (React.createElement("div", {className: "inactive-build"}, " inactive build "))
}

// A Task contains the information for a single task for a build, including the link to its page, and a tooltip
function Task({task}) {
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
function CollapsedBuild({build, validTasks}){
  var taskStats = build.taskStatusCount;

  var taskTypes = {
  "success"      : taskStats.succeeded, 
  "dispatched"   : taskStats.started, 
  "system-failed": taskStats.timed_out,
  "undispatched" : taskStats.undispatched, 
  "inactive"     : taskStats.inactive,
  "failed" :        taskStats.failed,
  };

  // Remove all task summaries that have 0 tasks
  taskTypes = _.pick(taskTypes, function(count, status){
    return count > 0 && !(_.contains(validTasks, status))
  });
  
  return (
    React.createElement("div", {className: "collapsed-bar"}, 
      
        _.map(taskTypes, function(count, status) {
          return React.createElement(TaskSummary, {key: status, status: status, count: count});
        }) 
      
    )
  )
}

// A TaskSummary is the class for one rolled up task type
// A CollapsedBuild is comprised of an  array of contiguous TaskSummaries below individual failing tasks 
function TaskSummary({status, count}){
  return (
    React.createElement("div", {className: status + " task-summary"}, 
      "+", count
    )
  )
}
