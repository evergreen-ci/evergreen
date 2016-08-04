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
    this.state = {collapsed: false};
    this.handleCollapseChange = this.handleCollapseChange.bind(this);
  }
  handleCollapseChange(collapsed) {
    this.setState({collapsed: collapsed});
  }
  render() {
    var toolbarData = {collapsed : this.state.collapsed,
                       onCheck : this.handleCollapseChange};

    return (
      <div> 
        <Toolbar data={this.props.data} collapsed={this.state.collapsed} onCheck={this.handleCollapseChange} toolbarData={toolbarData} stuff={"hellolol"} things={"nope"}/>
        <Headers versions={this.props.data.versions}/> 
        <Grid data={this.props.data} collapsed={this.state.collapsed} project={this.props.project} />
      </div>
    )
  }
}

/*** START OF WATERFALL TOOLBAR ***/

const Toolbar = ({toolbarData}) => {
  return (
    <CollapseButton collapsed={toolbarData.collapsed} onCheck={toolbarData.onCheck}  /> 
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
      <label style={{display:"inline-block"}}>
        <span style={{fontWeight:"normal"}}>Show Collapsed View </span><input style={{display:"inline"}}
          className="checkbox"
          type="checkbox"
          checked={this.props.collapsed}
          ref="collapsedBuilds"
          onChange={this.handleChange} />
      </label>
    )
  }
}

/*** START OF WATERFALL HEADERS ***/

function Headers ({versions}) {
  var versionList = _.sortBy(_.values(versions), 'revision_order').reverse();
  return (
  <div className="row version-header">
    <div className="variant-col col-xs-2 version-header-full text-right">
      Variant
    </div>
    {
      _.map(versionList, function(version){
        if (version.rolled_up) {
          return <RolledUpVersionHeader key={version.ids[0]} version={version} />
        }
        // Unrolled up version, no popover
        return <ActiveVersionHeader key={version.ids[0]} version={version} />;
      })
    }
    <br/>
  </div>
  )
}

function ActiveVersionHeader({version}) {
  
  var message = version.messages[0];
  var author = version.authors[0];
  var id_link = "/version/" + version.ids[0];
  var commit = version.revisions[0].substring(0,5);
  var message = version.messages[0]; 
  var shortened_message = version.messages[0].substring(0,35);

  var formatted_time = getFormattedTime(new Date(version.create_times[0]));

  return (
      <div className="col-xs-2">
        <div className="version-header-expanded">
          <div>
            <span className="btn btn-default btn-hash history-item-revision">
              <a href={id_link}>{commit}</a>
            </span>
            {formatted_time}
          </div>
          {author} - {shortened_message}
        </div>
      </div>
  )
};

function RolledUpVersionHeader({version}){
  var Popover = ReactBootstrap.Popover;
  var OverlayTrigger = ReactBootstrap.OverlayTrigger;
  var Button = ReactBootstrap.Button;
  
  var versionTitle = version.messages.length > 1 ? "versions" : "version";
  var rolledHeader = version.messages.length + " inactive " + versionTitle; 
 
  const popovers = (
    <Popover id="popover-positioned-bottom" title="">
      {
        version.ids.map(function(id,i) {
          return <RolledUpVersionSummary version={version} key={id} i={i} />
        })
      }
    </Popover>
  );

  return (
    <div className="col-xs-2">
      <OverlayTrigger trigger="click" placement="bottom" overlay={popovers} className="col-xs-2">
        <Button className="rolled-up-button">
          <a href="#">{rolledHeader}</a>
        </Button>
      </OverlayTrigger>
    </div>
  )
};

function RolledUpVersionSummary ({version, i}) {
  var formatted_time = getFormattedTime(new Date(version.create_times[i]));
  var author = version.authors[i];
  var commit =  version.revisions[i].substring(0,10);
  var message = version.messages[i];
    
  return (
    <div className="rolled-up-version-summary">
      <span className="version-header-time">{formatted_time}</span>
      <br /> 
      <a href={"/version/" + version.ids[i]}>{commit}</a> - <strong>{author}</strong> 
      <br /> 
      {message} 
      <br />
    </div>
  )
};

/*** START OF WATERFALL GRID ***/

// The main class that binds to the root div. This contains all the distros, builds, and tasks
function Grid ({data, project, collapsed}) {
  return (
    <div className="waterfall-grid">
      {
        data.rows.map(function(row){
          return <Variant row={row} project={project} collapsed={collapsed} versions={data.versions} />;
        })
      }
    </div> 
  )
};

// The class for each "row" of the waterfall page. Includes the build variant link, as well as the five columns
// of versions.
function Variant({row, versions, project, collapsed}) {
      return (
      <div className="row variant-row">
        <div className="col-xs-2 build-variant-name distro-col"> 
        <a href={"/build_variant/" + project + "/" + row.build_variant.id}>
            {row.build_variant.display_name}
          </a> 
        </div>
        <div className="col-xs-10"> 
          <div className="row build-cols">
            {
              row.versions.map((versionId,i) => {
                return <Build key={versionId} build={row.builds[versionId]} version={versions[versionId]} collapsed={collapsed} />
              })
            }
          </div>
        </div>
      </div>
    )
}


// Each Build class is one group of tasks for an version + build variant intersection
// We case on whether or not a build is active or not, and return either an ActiveBuild or InactiveBuild respectively
function Build({build, collapsed, version}){
  // inactive build
  if (version.rolled_up) {
    return <InactiveBuild className="build"/>;
  }
  // collapsed active build
  if (collapsed) {
    var validTasks = ['failed','system-failed']; // Can be modified to show combinations of tasks by statuses      
    return (
      <div className="build">
        <ActiveBuild build={build} validTasks={validTasks} />
        <CollapsedBuild build={build} validTasks={validTasks} />
      </div>
    )
  } 
  // uncollapsed active build
  return (
    <div className="build">
      <ActiveBuild build={build}/>
    </div>
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
    <div className="active-build"> 
      {
        tasks.map(function(task){
          return <Task task={task} />
        })
      }
    </div>
  )
}

// All tasks are inactive, so we display the words "inactive build"
function InactiveBuild ({}){
    return (<div className="inactive-build"> inactive build </div>)
}

// A Task contains the information for a single task for a build, including the link to its page, and a tooltip
function Task({task}) {
  var status = task.status;
  var tooltipContent = task.display_name + " - " + status;

  return (
    <div className="waterfall-box"> 
      <a href={"/task/" + task.id} className={"task-result " + status} />  
    </div>
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
    <div className="collapsed-bar">
      {
        _.map(taskTypes, function(count, status) {
          return <TaskSummary key={status} status={status} count={count} />;
        }) 
      }
    </div>
  )
}

// A TaskSummary is the class for one rolled up task type
// A CollapsedBuild is comprised of an  array of contiguous TaskSummaries below individual failing tasks 
function TaskSummary({status, count}){
  return (
    <div className={status + " task-summary"}> 
      +{count}
    </div>
  )
}
