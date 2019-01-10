mciModule.factory('PerfChartService', function() {
  var cfg = {
    container: {
      width: 960,
      height: 222
    },
    margin: {
      top: 12,
      right: 50,
      bottom: 60,
      left: 120
    },
    effectiveWidth: undefined, // to be calculated
    effectiveHeight: undefined, // to be calculated
    points: {
      focusedR: 4.5,
      changePointSize: 12,
    },
    toolTip: { // For change points
      interlineSpace: 20,
      width: 165,
      margin: 15,
      xOffset: -65,
      yOffset: 0,
      colOffset: [0, 45, 50, 65],
      height: undefined, // to be calculated
      labels: [{
        key: 'nobs',
        display: '#',
      }, {
        key: function(d) { return d.minmax[0] },
        display: 'Min',
      }, {
        key: function(d) { return d.minmax[1] },
        display: 'Max',
      }, {
        key: 'mean',
        display: 'Mean',
      }, {
        key: 'variance',
        display: 'Variance',
      }, {
        key: 'bfs',
        display: 'Tickets',
        nonStat: true,
        formatter: function(val) { return (
          val.length == 0 ? 'None' :
          val.length == 1 ? val[0].key :
          /* else */        val[0].key + ' and ' + val.length - 1 + ' more'
        )},
      }, {
        key: 'skewness',
        display: 'Skewness',
        visible: false,
      }, {
        key: 'kurtosis',
        display: 'Kurtosis',
        visible: false,
      }].filter(function(d) {
        // Filter out non-visible items
        return d.visible !== false
      }).map(function(d, i) {
        // Enumerate all items (required for rendering heterogeneous lists by d3js)
        d.idx = i
        return d
      }),
    },
    valueAttr: 'ops_per_sec',
    yAxis: {
      ticks: 5,
      gap: 10 // White space between axis and chart
    },
    xAxis: {
      maxTicks: 10,
      format: 'MMM DD',
      labelYOffset: 20,
    },
    focus: {
      labelOffset: {
        x: 6,
        y: -5,
        between: 15, // Minimal vertical space between ops labels
      }
    },
    format: {
      date: 'll'
    },
    legend: {
      itemHeight: 20,
      itemWidth: 35,
      gap: 10, // between legen items
      yOffset: 10, // To bootom
      textOverRectOffset: -15, // Aligns legend item with the text label
      yPos: undefined, // To be calculated
      xPos: undefined, // To be calculated
      step: undefined, // to e calculated. Step between legend items
    },
    knownLevels: {
        1: { colorId: 0 },
        2: { colorId: 9 },
        4: { colorId: 1 },
        8: { colorId: 2 },
       16: { colorId: 3 },
       32: { colorId: 4 },
       64: { colorId: 5 },
      128: { colorId: 8 },
      256: { colorId: 6 },
      512: { colorId: 7 },
    },
    formatters: {
      large: d3.format(',.0f'), // grouped thousands, no significant digits
      digits_1: d3.format('.01f'), // floating point 1 digits
      digits_2: d3.format('.02f'), // floating point 2 digits
      digits_3: d3.format('.03f'), // floating point 3 digits
      si: d3.format(',s'), // si notation
    }
  };

  // Non-persistent color id offset for unusual thread level
  cfg.knownLevelsCount = _.keys(cfg.knownLevels).length

  // Chart are real size
  cfg.effectiveWidth = cfg.container.width - cfg.margin.left - cfg.margin.right;
  cfg.effectiveHeight = cfg.container.height - cfg.margin.top - cfg.margin.bottom;

  // Tool Tip
  cfg.toolTip.height = cfg.toolTip.interlineSpace * cfg.toolTip.labels.length

  // Legend y pos
  cfg.legend.yPos = (
    cfg.container.height - cfg.legend.yOffset
    - cfg.legend.itemHeight - cfg.legend.textOverRectOffset
  )

  cfg.legend.step = cfg.legend.itemWidth + cfg.legend.gap

  // Returns list of y-positions of ops labels for given
  // yScaledValues list.
  function getOpsLabelYPosition(vals, cfg) {
    var yScaledValues = _.sortBy(vals, function(d) { return -d })

    // Calculate the most top (the last) label position.
    // Also checks top margin overlap
    var currentVal = _.last(yScaledValues)
    var prevPos = currentVal + cfg.focus.labelOffset.y
    if (prevPos < cfg.margin.top) {
      prevPos = cfg.margin.top + 5
    }
    var textPosList = []
    textPosList[_.indexOf(vals, currentVal)] = prevPos

    // Calculate all other items positions, based on previous item position
    // Loop skips the last item (see code above)
    for (var i = yScaledValues.length - 2; i >= 0; i--) {
      var currentVal = yScaledValues[i]
      var currentPos = currentVal + cfg.focus.labelOffset.y;
      var delta = prevPos - currentPos;
      // If labels overlapping, move the label below previous label
      var newPos = (delta > -cfg.focus.labelOffset.between)
        ? prevPos + cfg.focus.labelOffset.between
        : currentPos
      prevPos = newPos
      textPosList[_.indexOf(vals, currentVal)] = newPos
    }

    return textPosList;
  }

  // Pair for getValueForAllLevels function
  // This curried function returns value for given
  // thread `level` (ignored in this case) and series `item`
  function getValueForMaxOnly() {
    return function(item) {
      return _.last(item.threadResults)[cfg.valueAttr]
    }
  }

  // Pair for getValueForMaxOnly function
  // This curried function returns value for given
  // thread `level` and series `item`
  function getValueForAllLevels(level) {
    return function(item) {
      var result = _.findWhere(item.threadResults, {threadLevel: level.name})
      return result
        ? result[cfg.valueAttr]
        : null
    }
  }

  return {
    cfg: cfg,
    getOpsLabelYPosition: getOpsLabelYPosition,
    getValueForMaxOnly: getValueForMaxOnly,
    getValueForAllLevels: getValueForAllLevels,
  }
})
