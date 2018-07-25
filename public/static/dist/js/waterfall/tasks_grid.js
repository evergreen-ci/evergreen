'use strict';

var _createClass = function () { function defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } } return function (Constructor, protoProps, staticProps) { if (protoProps) defineProperties(Constructor.prototype, protoProps); if (staticProps) defineProperties(Constructor, staticProps); return Constructor; }; }();

function _possibleConstructorReturn(self, call) { if (!self) { throw new ReferenceError("this hasn't been initialised - super() hasn't been called"); } return call && (typeof call === "object" || typeof call === "function") ? call : self; }

function _inherits(subClass, superClass) { if (typeof superClass !== "function" && superClass !== null) { throw new TypeError("Super expression must either be null or a function, not " + typeof superClass); } subClass.prototype = Object.create(superClass && superClass.prototype, { constructor: { value: subClass, enumerable: false, writable: true, configurable: true } }); if (superClass) Object.setPrototypeOf ? Object.setPrototypeOf(subClass, superClass) : subClass.__proto__ = superClass; }

function _classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function _objectDestructuringEmpty(obj) { if (obj == null) throw new TypeError("Cannot destructure undefined"); }

var MaxFailedTestDisplay = 5;

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
function labelFromTask(task) {
  if (task !== Object(task)) {
    return '';
  }

  if (task.status == 'undispatched') {
    if (task.activated) {
      if (task.task_waiting) {
        return task.task_waiting;
      }
      return 'scheduled';
    } else if (+task.dispatch_time == 0 || typeof task.dispatch_time == "string" && +new Date(task.dispatch_time) <= 0) {
      return 'not scheduled';
    }
  }

  if (task.status == 'failed' && 'task_end_details' in task) {
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
  var NS_PER_SEC = NS_PER_MS * 1000;
  var NS_PER_MINUTE = NS_PER_SEC * 60;
  var NS_PER_HOUR = NS_PER_MINUTE * 60;

  if (input == 0) {
    return "0 seconds";
  } else if (input < NS_PER_MS) {
    return "< 1 ms";
  } else if (input < NS_PER_SEC) {
    if (skipSecMax) {
      return Math.floor(input / NS_PER_MS) + " ms";
    } else {
      return "< 1 second";
    }
  } else if (input < NS_PER_MINUTE) {
    return Math.floor(input / NS_PER_SEC) + " seconds";
  } else if (input < NS_PER_HOUR) {
    return Math.floor(input / NS_PER_MINUTE) + "m " + Math.floor(input % NS_PER_MINUTE / NS_PER_SEC) + "s";
  } else if (input < NS_PER_HOUR * 24 || skipDayMax) {
    return Math.floor(input / NS_PER_HOUR) + "h " + Math.floor(input % NS_PER_HOUR / NS_PER_MINUTE) + "m " + Math.floor(input % NS_PER_MINUTE / NS_PER_SEC) + "s";
  } else if (input == "unknown") {
    return "unknown";
  } else {
    return ">= 1 day";
  }
}

// Grid

// The main class that binds to the root div. This contains all the distros, builds, and tasks
function Grid(_ref) {
  var data = _ref.data,
      project = _ref.project,
      collapseInfo = _ref.collapseInfo,
      buildVariantFilter = _ref.buildVariantFilter,
      taskFilter = _ref.taskFilter;

  return React.createElement(
    'div',
    { className: 'waterfall-grid' },
    data.rows.filter(function (row) {
      return row.build_variant.display_name.toLowerCase().indexOf(buildVariantFilter.toLowerCase()) != -1;
    }).map(function (row) {
      return React.createElement(Variant, { row: row, project: project, collapseInfo: collapseInfo, versions: data.versions, taskFilter: taskFilter, currentTime: data.current_time });
    })
  );
};

function filterActiveTasks(tasks, activeStatuses) {
  return _.filter(tasks, function (task) {
    return _.contains(activeStatuses, task.status);
  });
}

// The class for each "row" of the waterfall page. Includes the build variant link, as well as the five columns
// of versions.
function Variant(_ref2) {
  var row = _ref2.row,
      versions = _ref2.versions,
      project = _ref2.project,
      collapseInfo = _ref2.collapseInfo,
      taskFilter = _ref2.taskFilter,
      currentTime = _ref2.currentTime;

  return React.createElement(
    'div',
    { className: 'row variant-row' },
    React.createElement(
      'div',
      { className: 'col-xs-2 build-variants' },
      row.build_variant.display_name
    ),
    React.createElement(
      'div',
      { className: 'col-xs-10' },
      React.createElement(
        'div',
        { className: 'row build-cells' },
        versions.map(function (version, i) {
          return React.createElement(
            'div',
            { className: 'waterfall-build' },
            React.createElement(Build, { key: version.ids[0],
              build: row.builds[version.ids[0]],
              rolledUp: version.rolled_up,
              collapseInfo: collapseInfo,
              taskFilter: taskFilter,
              currentTime: currentTime })
          );
        })
      )
    )
  );
}

// Each Build class is one group of tasks for an version + build variant intersection
// We case on whether or not a build is active or not, and return either an ActiveBuild or InactiveBuild respectively

function Build(_ref3) {
  var build = _ref3.build,
      collapseInfo = _ref3.collapseInfo,
      rolledUp = _ref3.rolledUp,
      taskFilter = _ref3.taskFilter,
      currentTime = _ref3.currentTime;

  // inactive build
  if (rolledUp) {
    return React.createElement(InactiveBuild, null);
  }

  // no build for this version
  if (!build) {
    return React.createElement(EmptyBuild, null);
  }

  // collapsed active build
  if (collapseInfo.collapsed) {
    activeTasks = filterActiveTasks(build.tasks, collapseInfo.activeTaskStatuses);
    if (activeTasks.length == 0) {
      return React.createElement(CollapsedBuild, { build: build, activeTaskStatuses: collapseInfo.activeTaskStatuses });
    }
    // Can be modified to show combinations of tasks by statuses  
    var activeTasks = filterActiveTasks(build.tasks, collapseInfo.activeTaskStatuses);
    return React.createElement(
      'div',
      null,
      React.createElement(CollapsedBuild, { build: build, activeTaskStatuses: collapseInfo.activeTaskStatuses }),
      React.createElement(ActiveBuild, { tasks: activeTasks, currentTime: currentTime })
    );
  }
  // uncollapsed active build
  return React.createElement(ActiveBuild, { tasks: build.tasks, taskFilter: taskFilter, currentTime: currentTime });
}

// At least one task in the version is not inactive, so we display all build tasks with their appropiate colors signifying their status
function ActiveBuild(_ref4) {
  var tasks = _ref4.tasks,
      taskFilter = _ref4.taskFilter,
      currentTime = _ref4.currentTime;


  if (taskFilter != null) {
    tasks = _.filter(tasks, function (task) {
      return task.display_name.toLowerCase().indexOf(taskFilter.toLowerCase()) != -1;
    });
  }

  return React.createElement(
    'div',
    { className: 'active-build' },
    _.map(tasks, function (task) {
      return React.createElement(Task, { task: task, currentTime: currentTime });
    })
  );
}

// All tasks are inactive, so we display the words "inactive build"
function InactiveBuild(_ref5) {
  _objectDestructuringEmpty(_ref5);

  return React.createElement(
    'div',
    { className: 'inactive-build' },
    ' inactive build '
  );
}
// No build associated with a given version and variant, so we render an empty div
function EmptyBuild(_ref6) {
  _objectDestructuringEmpty(_ref6);

  return React.createElement('div', null);
}

function TooltipContent(_ref7) {
  var task = _ref7.task,
      eta = _ref7.eta;

  var topLineContent = task.display_name + " - " + labelFromTask(task);
  if (task.status == 'success' || task.status == 'failed') {
    var dur = stringifyNanoseconds(task.time_taken);
    topLineContent += ' - ' + dur;
  }

  if (task.status != 'failed' || !task.failed_test_names || task.failed_test_names.length == 0) {
    if (task.status == 'started') {
      return React.createElement(
        'span',
        { className: 'waterfall-tooltip' },
        topLineContent,
        ' - ',
        eta
      );
    }
    return React.createElement(
      'span',
      { className: 'waterfall-tooltip' },
      topLineContent
    );
  }

  if (task.failed_test_names.length > MaxFailedTestDisplay) {
    return React.createElement(
      'span',
      { className: 'waterfall-tooltip' },
      React.createElement(
        'span',
        null,
        topLineContent
      ),
      React.createElement(
        'div',
        { className: 'header' },
        React.createElement('i', { className: 'fa fa-times icon' }),
        task.failed_test_names.length,
        ' failed tests'
      )
    );
  }
  return React.createElement(
    'span',
    { className: 'waterfall-tooltip' },
    React.createElement(
      'span',
      null,
      topLineContent
    ),
    React.createElement(
      'div',
      { className: 'failed-tests' },
      task.failed_test_names.map(function (failed_test_name) {
        return React.createElement(
          'div',
          null,
          React.createElement('i', { className: 'fa fa-times icon' }),
          endOfPath(failed_test_name)
        );
      })
    )
  );
}

// CountdownClock is a class that manages decrementing duration every second.
// It takes as an argument nanosecondsRemaining and begins counting this number
// down as soon as it is instantiated.

var CountdownClock = function () {
  function CountdownClock(nanosecondsRemaining) {
    _classCallCheck(this, CountdownClock);

    this.tick = this.tick.bind(this);
    this.countdown = setInterval(this.tick, 1000);
    this.nanosecondsRemaining = nanosecondsRemaining;
  }

  _createClass(CountdownClock, [{
    key: 'tick',
    value: function tick() {
      this.nanosecondsRemaining -= 1 * (1000 * 1000 * 1000);
      if (this.nanosecondsRemaining <= 0) {
        this.nanosecondsRemaining = 0;
        clearInterval(this.countdown);
      }
    }
  }, {
    key: 'getNanosecondsRemaining',
    value: function getNanosecondsRemaining() {
      return this.nanosecondsRemaining;
    }
  }]);

  return CountdownClock;
}();

// ETADisplay is a react component that manages displaying a time being
// counted down. It takes as a prop a CountdownClock, which it uses to fetch
// the time left in the count down.


var ETADisplay = function (_React$Component) {
  _inherits(ETADisplay, _React$Component);

  function ETADisplay(props) {
    _classCallCheck(this, ETADisplay);

    var _this = _possibleConstructorReturn(this, (ETADisplay.__proto__ || Object.getPrototypeOf(ETADisplay)).call(this, props));

    _this.tick = _this.tick.bind(_this);
    _this.componentWillUnmount = _this.componentWillUnmount.bind(_this);

    _this.update = setInterval(_this.tick, 1000);
    _this.countdownClock = _this.props.countdownClock;

    var nsString = stringifyNanoseconds(_this.countdownClock.getNanosecondsRemaining());

    if (_this.countdownClock.getNanosecondsRemaining() <= 0) {
      nsString = 'unknown';
    }
    _this.state = {
      ETAString: nsString
    };

    return _this;
  }

  _createClass(ETADisplay, [{
    key: 'tick',
    value: function tick() {
      var nsRemaining = this.countdownClock.getNanosecondsRemaining();
      var nsString = stringifyNanoseconds(nsRemaining);

      if (nsRemaining <= 0) {
        nsString = 'unknown';
        clearInterval(this.countdown);
      }
      this.setState({
        ETAString: nsString
      });
    }
  }, {
    key: 'componentWillUnmount',
    value: function componentWillUnmount() {
      clearInterval(this.interval);
    }
  }, {
    key: 'render',
    value: function render() {
      return React.createElement(
        'span',
        null,
        'ETA: ',
        this.state.ETAString
      );
    }
  }]);

  return ETADisplay;
}(React.Component);

// A Task contains the information for a single task for a build, including the link to its page, and a tooltip


function Task(_ref8) {
  var task = _ref8.task,
      currentTime = _ref8.currentTime;

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
    var eta = React.createElement(ETADisplay, { countdownClock: clock });
  }
  var tooltip = React.createElement(
    Tooltip,
    { id: 'tooltip' },
    React.createElement(TooltipContent, { task: task, eta: eta })
  );
  return React.createElement(
    OverlayTrigger,
    { placement: 'top', overlay: tooltip, animation: false },
    React.createElement('a', { href: "/task/" + task.id, className: "waterfall-box " + taskStatusClass(task) })
  );
}

// A CollapsedBuild contains a set of PartialProgressBars, which in turn make up a full progress bar
// We iterate over the 5 different main types of task statuses, each of which have a different color association
function CollapsedBuild(_ref9) {
  var build = _ref9.build,
      activeTaskStatuses = _ref9.activeTaskStatuses;

  var taskStats = build.taskStatusCount;

  var taskTypes = {
    "success": taskStats.succeeded,
    "dispatched": taskStats.started,
    "system-failed": taskStats.timed_out,
    "undispatched": taskStats.undispatched,
    "inactive": taskStats.inactive,
    "failed": taskStats.failed
  };

  // Remove all task summaries that have 0 tasks
  taskTypes = _.pick(taskTypes, function (count, status) {
    return count > 0 && !_.contains(activeTaskStatuses, status);
  });

  return React.createElement(
    'div',
    { className: 'collapsed-build' },
    _.map(taskTypes, function (count, status) {
      return React.createElement(TaskSummary, { status: status, count: count, build: build });
    })
  );
}

// A TaskSummary is the class for one rolled up task type
// A CollapsedBuild is comprised of an  array of contiguous TaskSummaries below individual failing tasks 
function TaskSummary(_ref10) {
  var status = _ref10.status,
      count = _ref10.count,
      build = _ref10.build;

  var id_link = "/build/" + build.id;
  var OverlayTrigger = ReactBootstrap.OverlayTrigger;
  var Popover = ReactBootstrap.Popover;
  var Tooltip = ReactBootstrap.Tooltip;
  var tt = React.createElement(
    Tooltip,
    { id: 'tooltip' },
    count,
    ' ',
    status
  );
  var classes = "task-summary " + status;
  return React.createElement(
    OverlayTrigger,
    { placement: 'top', overlay: tt, animation: false },
    React.createElement(
      'a',
      { href: id_link, className: classes },
      count
    )
  );
}
//# sourceMappingURL=tasks_grid.js.map
