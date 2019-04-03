describe('SettingsTest', function() {
  beforeEach(module('MCI'));

  var Settings

  beforeEach(inject(function($injector) {
    localStorage.clear()
    Settings = $injector.get('Settings')
  }))

  function typeCheck(typeDecl, value) {
    var tree = Settings._buildSettingsTree({
      param: {type: typeDecl}
    }, [])

    tree.param = value
    expect(tree.param).toEqual(value)
  }

  it('can store different param types', function() {
    typeCheck(Number, 1)
    typeCheck(Boolean, true)
    typeCheck(String, 'a')
    typeCheck(Object, {data: 'value'})
    typeCheck(undefined, 'a')
  })

  it('respect and restores deault value', function() {
    var tree = Settings._buildSettingsTree({
      param: {default: 'Default'}
    }, [])

    expect(tree.param).toBe('Default')
    // Ensure the value was set to localStorage
    expect(localStorage.getItem('param')).toBe('Default')
  })

  it('respect and restores deault value for Object type', function() {
    var tree = Settings._buildSettingsTree({
      param: {
        type: Object,
        default: {initial: 'value'},
      }
    }, [])

    expect(tree.param).toEqual({initial: 'value'})
  })

  it('actually uses localStorage', function() {
    var tree = Settings._buildSettingsTree({
      param: {type: String}
    }, [])

    expect(localStorage.getItem('param')).toBe(null)
    tree.param = 'v'
    expect(localStorage.getItem('param')).toBe('v')
    localStorage.setItem('param', 'other')
    expect(tree.param).toBe('other')
  })

  it('ensures all settings are known', function() {
    var knownSettings = [
      'perf.signalProcessing.persistentFiltering',
      'perf.trendchart.originMode.enabled',
      'perf.trendchart.linearMode.enabled',
      'perf.trendchart.rejectMode.enabled',
      'perf.trendchart.threadLevelMode',
    ]

    _.each(knownSettings, function(setting) {
      _.reduce(setting.split('.'), function(m, d) {
        expect(m[d]).toBeDefined('The setting "' + setting + '" does not exist!')
        return m[d]
      }, Settings)
    })
  })

  it('respects prefix', function() {
    var tree = Settings._buildSettingsTree({
      param: {type: String}
    }, ['prefix'])

    tree.param = 'a'
    expect(localStorage.getItem('prefix.param')).toBe('a')
  })

  it('returns undefined by default', function() {
    var tree = Settings._buildSettingsTree({
      param: {type: String}
    }, [])

    expect(tree.param).not.toBeDefined()
  })
})
