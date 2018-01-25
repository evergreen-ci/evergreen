describe('PerfChartServiceTest', function() {
  beforeEach(module('MCI'));
  var svc = null
  var cfg = null

  beforeEach(inject(function($injector) {
    svc = $injector.get('PerfChartService')
    cfg = svc.cfg
  }))

  it('should return valid config', function() {
    expect(
      _.keys(svc.cfg).length
    ).toBeGreaterThan(0)
  })

  it('keep orignial label positions when possible', function() {
    var yValues = [90, 60, 30]
    var expectedYValues = _.map(yValues, function(d) {
      return d + cfg.focus.labelOffset.y
    })

    expect(
      svc.getOpsLabelYPosition(yValues, cfg)
    ).toEqual(expectedYValues)
  })

  it('prevent labels overlap', function() {
    var yValues = [90, 89, 88]

    expect(
      d3.deviation(svc.getOpsLabelYPosition(yValues, cfg))
    ).toBeGreaterThan(d3.deviation(yValues))
  })

  it('extracts value for maxonly mode', function() {
    var item = {}
    item[cfg.valueAttr] = 1

    expect(
      svc.getValueForMaxOnly(null)(item)
    ).toBe(1)
  })

  it('extracts value for all levels mode', function() {
    var item = {threadResults: [{}, {}]}
    item[cfg.valueAttr] = 1
    item.threadResults[0][cfg.valueAttr] = 2
    item.threadResults[1][cfg.valueAttr] = 3

    // dummy thread levels array
    var levels = [{idx: 0}, {idx: 1}]

    expect(
      svc.getValueForAllLevels(levels[0])(item)
    ).toBe(2)

    expect(
      svc.getValueForAllLevels(levels[1])(item)
    ).toBe(3)
  })
});
