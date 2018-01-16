describe('PerfChartServiceTest', function() {
  beforeEach(module('MCI'));
  var PerfChartService = null

  beforeEach(inject(function($injector) {
    PerfChartService = $injector.get('PerfChartService')
  }))

  it('should return valid config', function() {
    expect(
      _.keys(PerfChartService.cfg).length
    ).toBeGreaterThan(0)
  })

  it('keep orignial label positions when possible', function() {
    var cfg = PerfChartService.cfg
    var yValues = [90, 60, 30]
    var expectedYValues = _.map(yValues, function(d) {
      return d + cfg.focus.labelOffset.y
    })

    expect(
      PerfChartService.getOpsLabelYPosition(yValues, cfg)
    ).toEqual(expectedYValues)
  })

  it('prevent labels overlap', function() {
    var cfg = PerfChartService.cfg
    var yValues = [90, 89, 88]

    expect(
      d3.deviation(PerfChartService.getOpsLabelYPosition(yValues, cfg))
    ).toBeGreaterThan(d3.deviation(yValues))
  })
});
