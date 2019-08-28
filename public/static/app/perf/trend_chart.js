// Helper function
// TODO Make it accept (d, i) and return function (FP)
function d3Translate(x, y) {
  if (y === undefined) y = x;
  return 'translate(' + x + ',' + y + ')';
}

// TODO Create AngularJS directive
mciModule.factory('DrawPerfTrendChart', function (
  PerfChartService, PROCESSED_TYPE) {
  const MAXONLY = 'maxonly';

  return function (params) {

    // Extract params
    const series = params.series,
      changePoints = params.changePoints,
      buildFailures = params.buildFailures,
      key = params.key,
      scope = params.scope,
      containerId = params.containerId,
      compareSamples = params.compareSamples,
      threadMode = params.threadMode,
      linearMode = params.linearMode,
      originMode = params.originMode,
      metric = params.metric;

    function idxByRevision(series, revision) {
      return _.findIndex(series, function (sample) {
        return sample && sample.revision === revision
      })
    }

    function hydrateChangePoint(point) {
      // if idx == 0, shift it right for one point
      const revIdx = idxByRevision(series, point.suspect_revision) || 1;

      point._meta = {
        firstRevIdx: revIdx - 1,
        lastRevIdx: revIdx,
      }
    }

    // Filter out change points which lays outside ot the chart
    let visibleChangePoints = (metric === "ops_per_sec") ? _.chain(changePoints)
      .filter(function (d) {
        return _.findWhere(series, {revision: d.suspect_revision})
      })
      .each(hydrateChangePoint) // Add some useful meta data
      .value() : [];

    const visibleBFs = _.filter(buildFailures, function (d) {
      return _.findWhere(series, {revision: d.first_failing_revision})
    });

    const cfg = PerfChartService.cfg;

    // override default metric
    cfg.valueAttr = metric;

    document.getElementById(containerId).innerHTML = '';

    const svg = d3.select('[id="' + containerId + '"]')
      .append('svg')
      .attr({
        class: 'series',
        width: cfg.container.width,
        height: cfg.container.height,
      });

    const colors = d3.scale.category10();
    // FIXME Force color range 'initialization'. Might be a bug in d3 3.5.3
    // For some reason, d3 will not give you, let's say, the third color
    // unless you requested 0, 1 and 2 before.
    for (var i = 0; i < cfg.knownLevelsCount; i++) colors(i);
    _.pluck(series, cfg.valueAttr);
    const opsValues = _.pluck(series, cfg.valueAttr + "_values");

    // Currently selcted revision item index
    const currentItemIdx = _.findIndex(series, function (d) {
      return d.task_id === scope.task.id
    });

    const allLevelsByIndex = series.map(entry => entry.threadResults.map(result => result.threadLevel));
    const allLevels = allLevelsByIndex.reduce((oneLevel, anotherLevel) => _.union(oneLevel, anotherLevel));
    const maxLevelByIndex = allLevelsByIndex.map(level => _.max(level));

    function maxLevelIdx(seriesIndex) {
      return series[seriesIndex].threadResults.map(result => result.threadLevel).indexOf(maxLevelByIndex[seriesIndex]);
    }

    maxIdx = _.max(allLevels);

    const levelsMeta = threadMode === MAXONLY
      ? [{name: 'MAX', idx: maxIdx, color: '#484848', isActive: true}]
      : _.map(allLevels, function (d, i) {
        const data = {
          name: d,
          idx: i,
          isActive: true,
        };

        const match = cfg.knownLevels[d];

        data.color = colors(match ? match.colorId : cfg.knownLevelsCount + i);
        return data
      });

    let activeLevels;
    let activeLevelNames;

    function updateActiveLevels() {
      activeLevels = levelsMeta.filter(meta => meta.isActive);
      activeLevelNames = activeLevels.map(level => level.name)
    }

    updateActiveLevels();

    // Depend on threadMode, key, activeLevelNames, cfg
    function getCompValues(compSample) {
      if (threadMode === MAXONLY) {
        return [compSample.maxThroughputForTest(key, cfg.valueAttr)]
      } else { // All thread levels mode
        const testResult = compSample.resultForTest(key);
        if (testResult) {
          return _.map(activeLevelNames, function (lvl) {
            let results = testResult.results[lvl];
            if (!results) {
              return null;
            }
            return results[cfg.valueAttr]
          })
        }
      }
    }

    // Convert compare samples into something that could
    // be rendered on the chart
    // Type: [[num, ...], ...]
    // First level array contain values for each baseline
    // Each value represent baseline value for thread level(s)
    let compSamplesValues = _.map(compareSamples, function (compSample) {
      return _.filter(getCompValues(compSample))
    });


    // For given `activeLevels` returns those which exists in the `sample`
    function threadLevelsForSample(sample, activeLevels) {
      return _.filter(activeLevels, function (d) {
        return _.includes(_.pluck(sample.threadResults, 'threadLevel'), d.name)
      })
    }

    const bfsForLevel = [];
    _.each(visibleBFs, function (bf) {
      _.each(levelsMeta, function (level) {
        // let allMatch = bfsForLevel.every(bfForLevel => bfForLevel.level === level && dbfForLevel.bf.first_failing_revision === bf.first_failing_revision)
        // if (!allMatch) {
        // TODO: reimplement this? Appears broken when discovered,
        //  commented out a better form of what *seems* right
        if (true) {
          bfsForLevel.push({
            level: level,
            bf: bf,
          })
        }
      })
    });

    // Array with combinations combinations of {level, changePoint}
    let changePointForLevel;

    function updateChangePointsForLevel() {
      changePointForLevel = [];
      _.each(visibleChangePoints, function (point) {
        point.bfs = _.where(
          buildFailures, {first_failing_revision: point.suspect_revision}
        ) || [];
        const level = _.findWhere(levelsMeta, {name: point.thread_level});
        const levels = level ? [level] : levelsMeta;

        _.each(levels, function (level) {
          // Check if there is existing point for this revision/level
          // Mostly for MAXONLY mode
          const existing = _.find(changePointForLevel, function (d) {
            return d.level === level && d.changePoint.suspect_revision === point.suspect_revision
          });
          // If the point already exists, increase count meta property
          if (existing) {
            existing.count++
          } else {
            changePointForLevel.push({
              level: level,
              changePoint: point,
              count: 1,
            })
          }
        })
      })
    }

    // Calculate legend x pos based on levels
    cfg.legend.xPos = (cfg.container.width - levelsMeta.length * cfg.legend.step) / 2;

    // Obtains value extractor fn for given `level` and series `item`
    // The obtained function is curried, so you should call it as fn(level)(item)
    const getValueFor = threadMode === MAXONLY
      ? PerfChartService.getValueForMaxOnly
      : PerfChartService.getValueForAllLevels;

    // When there are more than one value in opsValues item
    const hasValues = _.all(opsValues, function (d) {
      return d && d.length > 1
    });

    // Calculate X Ticks values
    const idxStep = (series.length / cfg.xAxis.maxTicks + 2) | 0;
    const xTicksData = _.filter(series, function (d, i) {
      return i % idxStep === 0
    });

    const xScale = d3.scale.linear()
      .domain([0, series.length - 1])
      .range([0, cfg.effectiveWidth]);

    let yScale = linearMode ? d3.scale.linear() : d3.scale.log();

    function getOpsValues(sample) {
      // For each active thread level extract values
      // including undefined values for missing data
      return _.map(activeLevelNames, function (d) {
        const result = _.findWhere(sample.threadResults, {threadLevel: d});
        return result && (result[cfg.valueAttr]);
      })
    }

    function calculateYScaleDomain() {
      const flatOpsValues = _.flatten(
        _.map(series, function (d, i) {
          if (threadMode === MAXONLY) {
            // In maxonly mode levels contain single (max) item
            // Extract just one ops item
            try {
              return d.threadResults[maxLevelIdx(i)][cfg.valueAttr];
            } catch (ex) {
              throw ex
            }
          } else {
            return getOpsValues(d);
          }
        })
      );

      let flatCompValues = _.flatten(compSamplesValues);
      let allValuesFlat = flatOpsValues.concat(flatCompValues);

      const multiSeriesAvg = d3.mean(allValuesFlat);
      const multiSeriesMin = d3.min(allValuesFlat);
      const multiSeriesMax = d3.max(allValuesFlat);

      // Zoomed mode / linear scale is default.
      // If the upper and lower y-axis values are very close to the average (within 10%)
      // add extra padding to the upper and lower bounds of the graph for display.
      let yAxisUpperBound = d3.max([multiSeriesMax, multiSeriesAvg * 1.1]);
      let yAxisLowerBound = originMode ? 0 : d3.min([multiSeriesMin, multiSeriesAvg * .9]);

      // Create a log based scale, remove any 0 values (log(0) is infinity).
      if (!linearMode) {
        if (yAxisUpperBound === 0) {
          yAxisUpperBound = 1e-1;
        }
        if (yAxisLowerBound === 0) {
          yAxisLowerBound = multiSeriesMin;
          if (yAxisLowerBound === 0) {
            yAxisLowerBound = 1e-1;
          }
        }
      }

      // We assume values are either all negative or all positive (around 0).
      // If the average is less than 0 then swap values and negate the
      // upper bound.
      if (multiSeriesAvg < 0) {
        if (!linearMode) {
          yAxisUpperBound = -yAxisUpperBound;
        }
        yAxisLowerBound = d3.min([multiSeriesMin, multiSeriesAvg * 1.1]);
      }

      return [yAxisLowerBound, yAxisUpperBound]
    }

    yScale = yScale.clamp(true)
      .range([cfg.effectiveHeight, 0])
      .nice(5);

    function updateYScaleDomain() {
      yScale.domain(calculateYScaleDomain())
    }

    updateYScaleDomain();

    const yAxis = d3.svg.axis()
      .scale(yScale)
      .orient('left')
      .ticks(cfg.yAxis.ticks, function (value) {
        return formatNumber(value)
      });

    // ## CHART STRUCTURE ##

    // multi line
    const mline = d3.svg.line()
      .defined(_.identity)
      .x(function (d, i) {
        return xScale(i);
      })
      .y(function (d) {
        return yScale(d);
      });

    if (hasValues) {
      var maxline = d3.svg.line()
        .defined(_.identity)
        .x(function (d, i) {
          return xScale(i)
        })
        .y(function (d) {
          return yScale(d3.max(d[cfg.valueAttr + "_values"]))
        });

      var minline = d3.svg.line()
        .defined(_.identity)
        .x(function (d, i) {
          return xScale(i)
        })
        .y(function (d) {
          return yScale(d3.min(d[cfg.valueAttr + "_values"]))
        })
    }

    // Y Axis
    svg.append('g')
      .attr({
        class: 'y-axis',
        transform: d3Translate(cfg.margin.left - cfg.yAxis.gap, cfg.margin.top)
      });

    function updateYAxis() {
      svg.select('g.y-axis')
        .transition()
        .call(yAxis)
    }

    updateYAxis();

    const getIdx = function (d) {
      return _.findIndex(series, d)
    };

    const xTicks = svg.append('svg:g')
      .attr({
        transform: d3Translate(cfg.margin.left, cfg.margin.top)
      })
      .selectAll('g')
      .data(xTicksData)
      .enter()
      .append('svg:g')
      .attr({
        transform: function (d) {
          return d3Translate(xScale(getIdx(d)), 0)
        }
      });

    // X Tick date text
    xTicks
      .append('svg:text')
      .attr({
        y: cfg.effectiveHeight + cfg.xAxis.labelYOffset,
        class: 'x-tick-label',
        'text-anchor': 'middle'
      })
      .text(function (d) {
        return moment(d.createTime).format(cfg.xAxis.format)
      });

    // X Tick vertical line
    xTicks
      .append('svg:line')
      .attr({
        class: 'x-tick-line',
        x0: 0,
        x1: 0,
        y1: 0,
        y2: cfg.effectiveHeight
      });

    // Show legend for 'all levels' mode only
    if (threadMode !== MAXONLY) {
      const legendG = svg.append('svg:g')
        .attr({
          class: 'legend',
          transform: d3Translate(cfg.legend.xPos, cfg.legend.yPos)
        });

      const legendIter = legendG.selectAll('g')
        .data(levelsMeta)
        .enter()
        .append('svg:g')
        .attr({
          transform: function (d, i) {
            return d3Translate(i * cfg.legend.step, 0)
          }
        })
        .style('cursor', 'pointer')
        .on('click', function (d) {
          d.isActive = !d.isActive;
          updateActiveLevels();
          updateYScaleDomain();
          updateYAxis();
          redrawLines();
          redrawLegend();
          redrawRefLines();
          redrawTooltip();
          redrawChangePoints();
          redrawBFs()
        });

      redrawLegend();

      function redrawLegend() {
        legendIter.append('svg:rect')
          .attr({
            y: cfg.legend.textOverRectOffset,
            width: cfg.legend.itemWidth,
            height: cfg.legend.itemHeight,
            fill: function (d) {
              return d.isActive ? d.color : '#666'
            }
          });

        legendIter.append('svg:text')
          .text(function (d) {
            return d.name
          })
          .attr({
            x: cfg.legend.itemWidth / 2,
            fill: function (d) {
              return d.isActive ? 'white' : '#DDD'
            },
            'text-anchor': 'middle',
          })
      }
    }

    // Chart draw area group
    const chartG = svg.append('svg:g')
      .attr('transform', d3Translate(cfg.margin.left, cfg.margin.top));

    const linesG = chartG.append('g').attr({class: 'lines-g'});

    function redrawLines() {
      const lines = linesG.selectAll('path')
        .data(activeLevels);

      lines
        .transition()
        .attr({
          d: function (level) {
            return mline(_.map(series, getValueFor(level)))
          }
        })
        .style({
          stroke: function (d) {
            return d.color
          },
        });

      lines
        .enter()
        .append('path')
        .attr({
          d: function (level) {
            return mline(_.map(series, getValueFor(level)))
          },
        })
        .style({
          stroke: function (d) {
            return d.color
          },
        });

      lines.exit().remove();

      // Only set the current revision marker if the current
      // revision is found
      if (currentItemIdx !== -1) {
        const commitCircle = chartG
          .selectAll('circle.current')
          .data(
            threadMode === MAXONLY
              ? activeLevels
              : threadLevelsForSample(series[currentItemIdx], activeLevels)
          );

        commitCircle
          .enter()
          .append('circle')
          .attr({
            class: 'point current',
            cx: xScale(currentItemIdx),
            cy: function (level) {
              return yScale(getValueFor(level)(series[currentItemIdx]))
            },
            r: cfg.points.focusedR + 0.5,
            stroke: function (d) {
              return d.color
            },
          });

        commitCircle
          .transition()
          .attr({
            cy: function (level) {
              return yScale(getValueFor(level)(series[currentItemIdx]))
            }
          });

        commitCircle.exit().remove()
      }
    }

    redrawLines();

    if (hasValues) {
      chartG.append('path')
        .data([series])
        .attr({
          class: 'error-line',
          d: maxline
        });

      chartG.append('path')
        .data([series])
        .attr({
          class: 'error-line',
          d: minline
        })
    }

    // Draws reference lines.
    // !! Should be executed in context of ref. line group only
    function drawRefLines(values, idx) {
      let refLines = d3.select(this)
        .selectAll('line')
        .data(values);

      // Create new elements (ref. lines)
      refLines
        .enter()
        .append('line')
        .attr({
          class: 'mean-line',
          x1: 0,
          x2: cfg.effectiveWidth,
          'stroke-width': '2',
          'stroke-dasharray': '5,5'
        });

      // Update existing elements (ref. lines)
      refLines
        .attr({
          y1: d => yScale(d),
          y2: function () {
            return this.attributes.y1.value
          }, // Tricky way to  y2 = y1
          stroke: function () {
            return d3.rgb(colors(idx + 1)).brighter()
          },
        });

      // Remove unused elements (ref. lines)
      refLines.exit().remove()
    }

    const refLinesTopLevelG = chartG.append('g').attr('class', 'cmp-lines');

    // top-level function which redraws reference lines
    // alias: baselines
    function redrawRefLines() {
      let refLinesG = refLinesTopLevelG.selectAll('g')
        .data(compSamplesValues);

      refLinesG
        .enter()
        .append('g');

      // Create new elements (ref. line groups)
      refLinesG
        .attr({class: (_, idx) => 'cmp-' + idx})
        .each(drawRefLines);

      // Remove unused elements (ref. line groups)
      refLinesG.exit().remove()
    }

    redrawRefLines();

    function redrawBFs() {
      const bfs = bfsG.selectAll('g.bf')
        .data(_.filter(bfsForLevel, function (d) {
          return d.level.isActive
        }));

      bfs
        .enter()
        .append('g')
        .attr({
          class: 'bf',
        })
        .append('path')
        .attr({
          d: 'M-5,0L0,8L5,0L0,-8Z', // Rhomb figure, marker for a BF
        })
        .style({
          stroke: 'green',
          fill: '#00FF00',
          'stroke-width': 2,
        });

      bfs
        .transition()
        .attr({
          transform: function (d) {
            const idx = idxByRevision(series, d.bf.first_failing_revision);
            return d3Translate(xScale(idx), yScale(getValueFor(d.level)(series[idx])))
          },
        });

      bfs.exit().remove()
    }

    function redrawChangePoints() {
      // Rebuild change points for level list before rendering
      updateChangePointsForLevel();

      // Render change points
      const pointsAndSegments = _.chain(changePointForLevel)
        .filter(function (d) {
          return d.level.isActive
        })
        .partition(function (d) { // Separate points and segments
          return d.changePoint._meta.firstRevIdx === d.changePoint._meta.lastRevIdx
        })
        .value();

      const changePointSegments = changePointsG.selectAll('g.change-point-segment')
        .data(pointsAndSegments[1]);

      changePointSegments
        .enter()
        .append('g')
        .attr({
          class: 'point change-point-segment',
        })
        .append('line')
        .style({
          'stroke-width': '4',
        });

      changePointSegments.select('line')
        .transition()
        .attr({
          x1: function (d) {
            return xScale(d.changePoint._meta.firstRevIdx)
          },
          x2: function (d) {
            return xScale(d.changePoint._meta.lastRevIdx)
          },
          y1: function (d) {
            return yScale(getValueFor(d.level)(series[d.changePoint._meta.firstRevIdx]))
          },
          y2: function (d) {
            return yScale(getValueFor(d.level)(series[d.changePoint._meta.lastRevIdx]))
          },
        })
        .style({
          stroke: (d) =>
            d.changePoint.processed_type === PROCESSED_TYPE.NONE ? 'red' :
              d.changePoint.processed_type === PROCESSED_TYPE.ACKNOWLEDGED ? 'green' :
                d.changePoint.processed_type === PROCESSED_TYPE.HIDDEN ? 'darkblue' :
                  'cyan' // cyan for invalid processed_type
          ,
        });

      changePointSegments.exit().remove();

      const changePointPoints = changePointsG.selectAll('g.change-point')
        .data(pointsAndSegments[0]);

      changePointPoints
        .enter()
        .append('g')
        .attr({
          class: 'point change-point',
        })
        .append('path') // Plus sign image for change points
        .attr({
          // plus-sign stroke
          class: 'change-point',
          d: 'M-7,3.5h4v4h6v-4h4v-6h-4v-4h-6v4h-4z',
        }).style({
        fill: function (d) {
          return d.count === 1 ? 'yellow' :
            d.count === 2 ? 'orange' :
              'red'
        },
      });

      changePointPoints
        .attr({
          transform: function (d) {
            const idx = _.findIndex(series, function (sample) {
              return sample && sample.revision === d.changePoint.suspect_revision
            });

            return idx > -1 ? d3Translate(
              xScale(idx),
              yScale(getValueFor(d.level)(series[idx]))
            ) : undefined
          },
        });

      changePointPoints.exit().remove()
    }

    var bfsG = chartG.append('g')
      .attr('class', 'g-bfs');

    redrawBFs();

    var changePointsG = chartG.append('g')
      .attr('class', 'g-change-points');

    redrawChangePoints();

    chartG.append('g')
      .attr({class: 'change-point-info'});

    // -- REGULAR POINT HOVER --
    // Contains elements for hover behavior
    const focusG = chartG.append('g')
      .style('display', 'none');

    const focusedLine = focusG.append('line')
      .attr({
        class: 'focus-line',
        x1: 0,
        x2: 0,
        y1: cfg.effectiveHeight,
      });

    let enableFocusGroup;
    let focusedPointsRef;
    let focusedTextRef;

    function redrawTooltip() {
      focusG.style('display', 'none');

      // This function could be called just once
      enableFocusGroup = _.once(
        function () {
          focusG.style('display', null)
        }
      )
    }

    redrawTooltip();

    function updateTooltip(levels) {
      const focusedPoints = focusG.selectAll('circle')
        .data(levels);

      focusedPointsRef = focusedPoints
        .attr({
          fill: function (d) {
            return d3.rgb(d.color).darker()
          },
        });

      focusedPoints
        .enter()
        .append('svg:circle')
        .attr({
          class: 'focus-point',
          r: cfg.points.focusedR,
          fill: function (d) {
            return d3.rgb(d.color).darker()
          },
        });

      focusedPoints.exit().remove();

      const focusedText = focusG.selectAll('text')
        .data(levels);

      focusedTextRef = focusedText
        .attr({
          fill: function (d) {
            return d3.rgb(d.color).darker(2)
          },
        });

      focusedText
        .enter()
        .append('svg:text')
        .attr({
          class: 'focus-text',
          x: cfg.focus.labelOffset.x,
          fill: function (d) {
            return d3.rgb(d.color).darker(2)
          },
        });

      focusedText.exit().remove()
    }

    // -- CHANGE POINT HOVER --
    const toolTipG = chartG.append('g')
      .attr({
        class: 'g-tool-tip',
      })
      .style('opacity', 0);

    const toolTipContentG = toolTipG.append('g')
      .attr({
        transform: d3Translate(
          -cfg.toolTip.xOffset + cfg.toolTip.margin,
          cfg.toolTip.yOffset + cfg.toolTip.margin
        ),
      });

    toolTipContentG.append('rect')
      .attr({
        x: cfg.toolTip.xOffset - cfg.toolTip.margin,
        y: cfg.toolTip.yOffset - cfg.toolTip.margin,
        width: cfg.toolTip.width + cfg.toolTip.margin * 2,
        height: cfg.toolTip.height + cfg.toolTip.margin * 2,
        rx: 5,
        ry: 5,
      })
      .style({
        fill: '#FFE',
        stroke: '#EED',
        'stroke-width': 2,
      });

    toolTipContentG.append('line')
      .attr({
        x1: 0,
        y1: 0,
        x2: 0,
        y2: cfg.toolTip.height,
      })
      .style({
        stroke: 'black',
        'stroke-width': 1,
      });

    function translateToolTipItem(d) {
      return d3Translate(0, cfg.toolTip.interlineSpace * (d.idx + .67))
    }

    const toolTipItem = toolTipContentG
      .selectAll('g.stat')
      // Display all statistical items
      // Non-statistical items are handled separately
      .data(
        _.filter(cfg.toolTip.labels, function (d) {
            return d.nonStat !== true
          }
        )
      )
      .enter()
      .append('g')
      .attr({
        class: 'stat',
        transform: translateToolTipItem,
      });

    const nonStatToolTipItem = toolTipContentG
      .selectAll('g.non-stat')
      .data(_.where(cfg.toolTip.labels, {nonStat: true}))
      .enter()
      .append('g')
      .attr({
        class: 'non-stat',
        transform: translateToolTipItem,
      });

    function placeTitle(d) {
      return d
        .append('text')
        .attr({
          x: -5,
          y: 0,
          'text-anchor': 'end',
        })
        .text(_.property('display'))
        .style({
          'font-weight': 'bold',
        })
    }

    toolTipItem.call(placeTitle);
    nonStatToolTipItem.call(placeTitle);

    toolTipItem
      .append('text')
      .attr({
        x: cfg.toolTip.colOffset[1],
        y: 0,
        'text-anchor': 'end',
        class: 'prev',
      });

    toolTipItem
      .append('text')
      .attr({
        x: cfg.toolTip.colOffset[2],
        y: 0,
      })
      .text('➞');

    toolTipItem
      .append('text')
      .attr({
        x: cfg.toolTip.colOffset[3],
        y: 0,
        class: 'next',
      });

    nonStatToolTipItem
      .append('text')
      .attr({
        x: cfg.toolTip.colOffset[0] + 5,
        y: 0,
        class: 'value',
      });

    // For given 1d directional segment with length `len` and offset `pos`
    // calculate new position, which fits `domain`
    // :param domain: domain we want to fit in
    // :ptype domain: [min, max]
    // :param pos: position of directional segment (actually 1d vector)
    // :param len: length of directional segment, non-negative
    // :param mirror: when true, and when the segment doesn't fit, mirrors the segment
    //                Domain: |───────────|
    //                Vec:    |        [──┼──>
    //                Ret:    |   <────]  |
    // :returns: new position of directional segment
    // Example:
    // A. domain=[0, 100], pos=50, len=10, mirror=false
    //    The segment perfectly fits domain, pos = 50
    //      Domain: |───────────|
    //      Vec:    |     [─>   |
    //      Ret:    |     [─>   |
    // B. domain=[0, 100], pos=-20, len=10, mirror=false
    //    The segment position is outside of domain, pos = 0, mirror=false
    //      Domain:       |───────────|
    //      Vec:     [──> |           |
    //      Ret:          ├─>         |
    // C. domain=[0, 100], pos=99, len=10
    //    The segment end position is outside of domain, pos = 90, mirror=false
    //      Domain: |───────────|
    //      Vec:    |          [┼─>
    //      Ret:    |       [──>|
    function fit1d(domain, pos, len, mirror) {
      if (pos < domain[0]) {
        return 0
      } else if (pos + len > domain[1]) {
        if (mirror) {
          return pos - len
        } else {
          return domain[1] - len
        }
      } else {
        return pos
      }
    }

    function fit(domain2d, pos2d, len2d) {
      return [
        fit1d(domain2d[0], pos2d[0], len2d[0], true),
        fit1d(domain2d[1], pos2d[1], len2d[1], false),
      ]
    }

    function changePointMouseMove() {
      // Fit tool tip panel into chart container
      toolTipG
        .attr({
          'transform': function () {
            const mousePos = d3.mouse(chartG[0][0]); // [0][0] is required to get bare <g> element
            return d3Translate.apply(
              null,
              fit(
                [[0, cfg.effectiveWidth], [0, cfg.effectiveHeight]],
                [mousePos[0] + 10, mousePos[1]], // 10 for extra offset
                [
                  cfg.toolTip.width + cfg.toolTip.margin * 2 + 20, // 20 for extra offset
                  cfg.toolTip.height
                ]
              )
            )
          }
        })
    }

    function changePointMouseOver(point) {
      function extractToolTipValue(item, obj) {
        // Get raw value of the stat item
        const rawValue = _.isFunction(item.key)
          ? item.key(obj)
          : obj[item.key];
        // Apply formatting if defined
        return item.formatter ? item.formatter(rawValue) : rawValue
      }

      function statTextFor(statItemName) {
        return function (d) {
          const cpStat = point.statistics[statItemName];
          return extractToolTipValue(d, cpStat)
        }
      }

      toolTipItem.select('text.prev')
        .text(_.compose(formatNumber, statTextFor('previous')));

      toolTipItem.select('text.next')
        .text(_.compose(formatNumber, statTextFor('next')));

      nonStatToolTipItem.select('text.value')
        .text(function (d) {
          return extractToolTipValue(d, point)
        });

      toolTipG
        .transition()
        .style('opacity', .9)
    }

    function changePointMouseOut() {
      toolTipG
        .transition()
        .style('opacity', 0)
    }

    function formatNumber(value) {
      return numeral(value).format("0.000a");
    }

    // Overlay to handle hover action
    chartG.append('svg:rect')
      .attr({
        class: 'overlay',
        width: cfg.effectiveWidth,
        height: cfg.effectiveHeight
      })
      .on('mouseover', function () {
        scope.currentHoverSeries = series;
      })
      .on('click', function () {
        scope.locked = !scope.locked;
        scope.$digest()
      })
      .on('mousemove', overlayMouseMove);

    let cpHover = false;

    function overlayMouseMove() {
      if (scope.locked) return;
      const idx = Math.round(xScale.invert(d3.mouse(this)[0]));
      const d = series[idx];

      // Handle the case where there is no data (it may be rejected).
      if (!d) {
        return;
      }
      const hash = d.revision;

      const hoveredChangePoint = _.filter(visibleChangePoints, function (d) {
        return d._meta.firstRevIdx <= idx && idx <= d._meta.lastRevIdx
      })[0];

      if (hoveredChangePoint) {
        if (!cpHover) {
          changePointMouseOver(hoveredChangePoint)
        }
        changePointMouseMove();
        cpHover = true
      } else if (cpHover) {
        changePointMouseOut();
        cpHover = false
      }

      // Reduce number of calls if hash didn't changed
      if (hash !== scope.$parent.currentHash) {
        scope.$parent.currentHash = hash;
        scope.$parent.currentHashDate = d.createTime;
        scope.$parent.bfs = _.pluck(
          _.where(visibleBFs, {first_failing_revision: hash}),
          'key'
        );
        scope.$parent.cps = _.filter(visibleChangePoints, function (d) {
          return d._meta.firstRevIdx === idx || d._meta.lastRevIdx === idx
        });
        scope.$emit('hashChanged', hash);
        scope.$parent.$digest()
      }
    }

    function focusPoint(hash) {
      const idx = _.findIndex(series, function (d) {
        return d && d.revision === hash
      });
      if (idx === undefined) return;
      const item = series[idx];
      if (!item) return;

      let x = xScale(idx);

      // List of per thread level values for selected item
      let values = _.filter(
        threadMode === MAXONLY
          ? [item.threadResults[maxLevelIdx(idx)][cfg.valueAttr]]
          : _.filter(getOpsValues(item))
      );

      // If there are no values for current metric, skip
      if (values.length === 0) return;

      const maxOps = _.max(values);

      // List of dot Y positions
      const yScaledValues = _.map(values, yScale);
      const opsLabelsY = PerfChartService.getOpsLabelYPosition(yScaledValues, cfg);

      focusG.attr('transform', d3Translate(x, 0));

      // Add/Remove tooltip items (some data samples may not contain all thread levels)
      updateTooltip(threadMode === MAXONLY
        ? activeLevels
        : threadLevelsForSample(item, activeLevels)
      );

      focusedPointsRef.attr({
        cy: function (d, i) {
          return yScaledValues[i]
        },
      });

      focusedTextRef
        .attr({
          y: function (d, i) {
            return opsLabelsY[i]
          },
          transform: function (d, i) {
            // transform the hover text location based on the list index
            let x = 0;

            // Catch NS_LOOKUP_ERROR. Checking `this` and `this.getBBox` don't work
            // FIXME Should be a better way. Probably, complete refactoring required
            try {
              this.getBBox()
            } catch (e) {
              return
            }

            // this.getBBox might not exist when the sample doesn't exist
            if (series && this.getBBox) {
              x = (cfg.focus.labelOffset.x + this.getBBox().width) * idx / series.length
            }
            return d3Translate(-x, 0)
          }
        })
        .text(function (d, i) {
          return formatNumber(values[i])
        });

      focusedLine.attr({
        y2: yScale(maxOps),
      })
    }

    // :ptype context: {pointRevs: Array<String>, processed_type: PROCESSED_TYPE}
    scope.$parent.$on('changePointsUpdate', function (evt, context) {
      // Assign new processed_type to visible change points
      _.each(visibleChangePoints, function (cp) {
        if (_.contains(context.pointRevs, cp.suspect_revision)) {
          cp.processed_type = context.processed_type
        }
      });

      redrawChangePoints();
      // Hide a tooltip
      toolTipG.style('opacity', 0);
      // Unlock the pointer
      scope.locked = false
    });

    scope.$on('hashChanged', function (evt, hash) {
      // Make tool tip visible
      enableFocusGroup();
      // Apply new position to tool tip
      focusPoint(hash)
    })
  }
});
