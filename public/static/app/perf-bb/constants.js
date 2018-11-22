mciModule
  .constant('PROCESSED_TYPE', (function() {
    var acknowledged = 'acknowledged'
    var hidden = 'hidden'
    return {
      ACKNOWLEDGED: acknowledged,
      HIDDEN:       hidden,
      NONE:         undefined,
      ALL:          [acknowledged, hidden]
    }
  })())
  .constant('PERFBB_HAZARD_VALUES', [
    'Major Regression',
    'Moderate Regression',
    'Minor Regression',
    'Minor Improvement',
    'Moderate Improvement',
    'Major Improvement',
  ])
  .constant('CHANGE_POINTS_GRID', {
    'HAZARD_COL_WIDTH': 125,
    'HAZARD_CHART_WIDTH': 60,
    'HAZARD_CHART_HEIGHT': 22,
  })
