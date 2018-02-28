const MaxFailedTestDisplay = 5;

// endOfPath strips off all of the begging characters from a file path so that just the file name is left.
function endOfPath(input) {
  var lastSlash = input.lastIndexOf('/');
  if (lastSlash === -1 || lastSlash === input.length - 1) {
    // try to find the index using windows-style filesystem separators
    lastSlash = input.lastIndexOf('\\');
    if (lastSlash === -1 || lastSlash === input.length - 1) {
      return input;
    }
  }
  return input.substring(lastSlash + 1);
}

// taskStatusClass returns the css class that should be associated with a given task so that it can
// be properly styled.
function taskStatusClass(task) {
  if (task !== Object(task)) {
	  return '';
  }

  if (task.status == 'undispatched') {
    if (!task.activated) {
      return 'inactive';
    } else {
      return 'unstarted';
    }
  }

  if (task.status == 'failed') {
    if ('task_end_details' in task) {
      if ('type' in task.task_end_details && task.task_end_details.type == 'setup') {
         return 'setup-failed';
      }
      if ('type' in task.task_end_details && task.task_end_details.type == 'system') {
         return 'system-failed';
      }
      if ('type' in task.task_end_details && task.task_end_details.type == 'setup') {
         return 'setup-failed';
      }
      if (!!task.task_end_details.timed_out && task.task_end_details.desc == 'heartbeat') {
        return 'system-failed';
      }
    }
    return 'failed';
  }
  return task.status;
}

// labelFromTask returns the human readable label for a task's status given the details of its execution.
function labelFromTask(task){
  if (task !== Object(task)) {
	  return '';
  }

  if (task.status == 'undispatched') {
    if (task.activated) {
      if (task.task_waiting) {
        return task.task_waiting;
      }
      return 'scheduled';
    } else if (+task.dispatch_time == 0 || (typeof task.dispatch_time == "string" && +new Date(task.dispatch_time) <= 0)) {
       return 'not scheduled';
    }
  }

  if (task.status == 'failed' && 'task_end_details' in task){
    if ('timed_out' in task.task_end_details) {
      if (task.task_end_details.timed_out && task.task_end_details.desc == 'heartbeat') {
        return 'system unresponsive';
      }
      if (task.task_end_details.type == 'system') {
        return 'system timed out';
      }
      return 'test timed out';
    }
    if (task.task_end_details.type == 'system') {
      return 'system failure';
    }
    if (task.task_end_details.type == 'setup') {
      return 'setup failure';
    }
  }

  return task.status;
}

// stringifyNanoseconds takes an integer count of nanoseconds and
// returns it formatted as a human readable string, like "1h32m40s"
// If skipDayMax is true, then durations longer than 1 day will be represented
// in hours. Otherwise, they will be displayed as '>=1 day'
function stringifyNanoseconds(input, skipDayMax, skipSecMax) {
  var NS_PER_MS = 1000 * 1000; // 10^6
  var NS_PER_SEC = NS_PER_MS * 1000
  var NS_PER_MINUTE = NS_PER_SEC * 60;
  var NS_PER_HOUR = NS_PER_MINUTE * 60;

  if (input == 0) {
    return "0 seconds";
  } else if (input < NS_PER_MS) {
    return "< 1 ms";
  } else if (input < NS_PER_SEC) {
    if (skipSecMax){
      return Math.floor(input / NS_PER_MS) + " ms";
    } else {
      return "< 1 second"
    }
  } else if (input < NS_PER_MINUTE) {
    return Math.floor(input / NS_PER_SEC) + " seconds";
  } else if (input < NS_PER_HOUR) {
    return Math.floor(input / NS_PER_MINUTE) + "m " + Math.floor((input % NS_PER_MINUTE) / NS_PER_SEC) + "s";
  } else if (input < NS_PER_HOUR * 24 || skipDayMax) {
    return Math.floor(input / NS_PER_HOUR) + "h " +
        Math.floor((input % NS_PER_HOUR) / NS_PER_MINUTE) + "m " +
        Math.floor((input % NS_PER_MINUTE) / NS_PER_SEC) + "s";
  } else if (input == "unknown") {
    return "unknown";
  }  else {
    return ">= 1 day";
  }
}

// Grid

// The main class that binds to the root div. This contains all the distros, builds, and tasks
function Grid ({data, project, collapseInfo, buildVariantFilter, taskFilter}) {
  return (
    React.createElement("div", {className: "waterfall-grid"}, 
      
        data.rows.filter(function(row){
          return row.build_variant.display_name.toLowerCase().indexOf(buildVariantFilter.toLowerCase()) != -1;
        })
        .map(function(row){
          return React.createElement(Variant, {row: row, project: project, collapseInfo: collapseInfo, versions: data.versions, taskFilter: taskFilter, currentTime: data.current_time});
        })
      
    ) 
  )
};

function filterActiveTasks(tasks, activeStatuses){
  return _.filter(tasks, function(task) { 
      return _.contains(activeStatuses, task.status);
    });
}

// The class for each "row" of the waterfall page. Includes the build variant link, as well as the five columns
// of versions.
function Variant({row, versions, project, collapseInfo, taskFilter, currentTime}) {
      return (
      React.createElement("div", {className: "row variant-row"}, 
        React.createElement("div", {className: "col-xs-2 build-variants"}, 
          row.build_variant.display_name
        ), 
        React.createElement("div", {className: "col-xs-10"}, 
          React.createElement("div", {className: "row build-cells"}, 
            
              versions.map(function(version, i){
                  return(React.createElement("div", {className: "waterfall-build"}, 
                    React.createElement(Build, {key: version.ids[0], 
                                build: row.builds[version.ids[0]], 
                                rolledUp: version.rolled_up, 
                                collapseInfo: collapseInfo, 
                                taskFilter: taskFilter, 
                                currentTime: currentTime})
                  )
                  );
              })
            
          )
        )
      )
    )
}


// Each Build class is one group of tasks for an version + build variant intersection
// We case on whether or not a build is active or not, and return either an ActiveBuild or InactiveBuild respectively

function Build({build, collapseInfo, rolledUp, taskFilter, currentTime}){
  // inactive build
  if (rolledUp) {
    return React.createElement(InactiveBuild, null);
  }

  // no build for this version
  if (!build) {
    return React.createElement(EmptyBuild, null)  
  }


  // collapsed active build
  if (collapseInfo.collapsed) {
    activeTasks = filterActiveTasks(build.tasks, collapseInfo.activeTaskStatuses);
    if (activeTasks.length == 0){
      return (
        React.createElement(CollapsedBuild, {build: build, activeTaskStatuses: collapseInfo.activeTaskStatuses})
      )
    }
    // Can be modified to show combinations of tasks by statuses  
    var activeTasks = filterActiveTasks(build.tasks, collapseInfo.activeTaskStatuses)
    return (
      React.createElement("div", null, 
        React.createElement(CollapsedBuild, {build: build, activeTaskStatuses: collapseInfo.activeTaskStatuses}), 
        React.createElement(ActiveBuild, {tasks: activeTasks, currentTime: currentTime})
      )
    )
  } 
  // uncollapsed active build
  return (
      React.createElement(ActiveBuild, {tasks: build.tasks, taskFilter: taskFilter, currentTime: currentTime})
  )
}

// At least one task in the version is not inactive, so we display all build tasks with their appropiate colors signifying their status
function ActiveBuild({tasks, taskFilter, currentTime}){  

  if (taskFilter != null){
    tasks = _.filter(tasks, function(task){
      return task.display_name.toLowerCase().indexOf(taskFilter.toLowerCase()) != -1;
    });
  }

  return (
    React.createElement("div", {className: "active-build"}, 
      
        _.map(tasks, function(task){
          return React.createElement(Task, {task: task, currentTime: currentTime})
        })
      
    )
  )
}

// All tasks are inactive, so we display the words "inactive build"
function InactiveBuild ({}){
    return (React.createElement("div", {className: "inactive-build"}, " inactive build "))
}
// No build associated with a given version and variant, so we render an empty div
function EmptyBuild ({}){
    return (React.createElement("div", null))
}

function TooltipContent({task, eta}) {
  var topLineContent = task.display_name + " - " + labelFromTask(task);
  if (task.status == 'success' || task.status == 'failed') {
    var dur = stringifyNanoseconds(task.time_taken);
    topLineContent += ' - ' + dur;
  }

  if (task.status !='failed' || !task.failed_test_names || task.failed_test_names.length == 0) {
    if (task.status == 'started') {
      return(
        React.createElement("span", {className: "waterfall-tooltip"}, 
          topLineContent, " - ", eta
        )
        )
    }
    return (
        React.createElement("span", {className: "waterfall-tooltip"}, 
          topLineContent
        )
        )
  }

  if (task.failed_test_names.length > MaxFailedTestDisplay) {
    return (
        React.createElement("span", {className: "waterfall-tooltip"}, 
          React.createElement("span", null, topLineContent), 
        React.createElement("div", {className: "header"}, 
          React.createElement("i", {className: "fa fa-times icon"}), 
          task.failed_test_names.length, " failed tests" 
          )
       )
        )
  }
  return(
      React.createElement("span", {className: "waterfall-tooltip"}, 
        React.createElement("span", null, topLineContent), 
      React.createElement("div", {className: "failed-tests"}, 
        
          task.failed_test_names.map(function(failed_test_name){
            return (
                React.createElement("div", null, 
                 React.createElement("i", {className: "fa fa-times icon"}), 
                  endOfPath(failed_test_name)
                )
                )
          })
        
        )
        )
      )
}

// CountdownClock is a class that manages decrementing duration every second.
// It takes as an argument nanosecondsRemaining and begins counting this number
// down as soon as it is instantiated.
class CountdownClock {
  constructor(nanosecondsRemaining) {
    this.tick = this.tick.bind(this);
    this.countdown = setInterval(this.tick, 1000);
    this.nanosecondsRemaining = nanosecondsRemaining;
  }
  tick() {
    this.nanosecondsRemaining -= 1 * (1000 * 1000 * 1000);
    if (this.nanosecondsRemaining <= 0) {
      this.nanosecondsRemaining = 0;
      clearInterval(this.countdown);
    }
  }
  getNanosecondsRemaining() {
    return this.nanosecondsRemaining;
  }
}

// ETADisplay is a react component that manages displaying a time being
// counted down. It takes as a prop a CountdownClock, which it uses to fetch
// the time left in the count down.
class ETADisplay extends React.Component {
  constructor(props) {
    super(props);
    this.tick = this.tick.bind(this);
    this.componentWillUnmount = this.componentWillUnmount.bind(this);

    this.update = setInterval(this.tick, 1000);
    this.countdownClock = this.props.countdownClock;

    var nsString = stringifyNanoseconds(this.countdownClock.getNanosecondsRemaining());

    if (this.countdownClock.getNanosecondsRemaining() <= 0) {
      nsString = 'unknown';
    }
    this.state = {
      ETAString: nsString
    };

  }

  tick() {
    var nsRemaining = this.countdownClock.getNanosecondsRemaining();
    var nsString = stringifyNanoseconds(nsRemaining);

    if (nsRemaining <= 0) {
      nsString = 'unknown';
      clearInterval(this.countdown);
    }
    this.setState({
      ETAString: nsString,
    });
  }

  componentWillUnmount() {
    clearInterval(this.interval);
  }
  render() {
    return (React.createElement("span", null, "ETA: ", this.state.ETAString));
  }
}


// A Task contains the information for a single task for a build, including the link to its page, and a tooltip
function Task({task, currentTime}) {
  var OverlayTrigger = ReactBootstrap.OverlayTrigger;
  var Popover = ReactBootstrap.Popover;
  var Tooltip = ReactBootstrap.Tooltip;
  var eta;
  if (task.status == 'started') {
    // If currentTime is not set, we're using the angular/react shim in
    // directives.visualization.js. We therefore need to set currentTime and
    // convert start_time to what react expects.  This is ugly, and long-term
    // our strategy should be to rewrite the version page in pure react.
    if (currentTime === undefined) {
      currentTime = new Date().getTime() * Math.pow(1000, 2);
      task.start_time = Date.parse(task.start_time) * Math.pow(1000, 2);
    }

    var timeRemaining = task.expected_duration - (currentTime - task.start_time);
    var clock = new CountdownClock(timeRemaining);
    var eta = (React.createElement(ETADisplay, {countdownClock: clock}));
  }
  var tooltip = (
      React.createElement(Tooltip, {id: "tooltip"}, 
        React.createElement(TooltipContent, {task: task, eta: eta})
      )
      )
  return (
    React.createElement(OverlayTrigger, {placement: "top", overlay: tooltip, animation: false}, 
      React.createElement("a", {href: "/task/" + task.id, className: "waterfall-box " + taskStatusClass(task)})
    )
  )
}

// A CollapsedBuild contains a set of PartialProgressBars, which in turn make up a full progress bar
// We iterate over the 5 different main types of task statuses, each of which have a different color association
function CollapsedBuild({build, activeTaskStatuses}){
  var taskStats = build.taskStatusCount;

  var taskTypes = {
    "success"      : taskStats.succeeded, 
    "dispatched"   : taskStats.started, 
    "system-failed": taskStats.timed_out,
    "undispatched" : taskStats.undispatched, 
    "inactive"     : taskStats.inactive,
    "failed"       : taskStats.failed,
  };

  // Remove all task summaries that have 0 tasks
  taskTypes = _.pick(taskTypes, function(count, status){
    return count > 0 && !(_.contains(activeTaskStatuses, status))
  });
  
  return (
    React.createElement("div", {className: "collapsed-build"}, 
      
        _.map(taskTypes, function(count, status) {
          return React.createElement(TaskSummary, {status: status, count: count, build: build});
        }) 
      
    )
  )
}

// A TaskSummary is the class for one rolled up task type
// A CollapsedBuild is comprised of an  array of contiguous TaskSummaries below individual failing tasks 
function TaskSummary({status, count, build}){
  var id_link = "/build/" + build.id;
  var OverlayTrigger = ReactBootstrap.OverlayTrigger;
  var Popover = ReactBootstrap.Popover;
  var Tooltip = ReactBootstrap.Tooltip;
  var tt = React.createElement(Tooltip, {id: "tooltip"}, count, " ", status);
  var classes = "task-summary " + status
  return (
    React.createElement(OverlayTrigger, {placement: "top", overlay: tt, animation: false}, 
      React.createElement("a", {href: id_link, className: classes}, 
        count
      )
    )
  )
}
