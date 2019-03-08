mciModule
  .constant('PROCESSED_TYPE', (function() {
    const acknowledged = 'acknowledged';
    const hidden = 'hidden';
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
  .constant('OUTLIERS_TYPE', {
    DETECTED: 'detected',
    SUSPICIOUS: 'suspicious',
  });
