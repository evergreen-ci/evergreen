/*
  Wrapper over window.localStorage
  - Allows to work with localStorage as with simple JS object
  - Auto type casting: String (default), Number, Boolean
  - Default setting value (undefined by default)

  Require setting to have defenition in the `constants`.
  The settings definition object is a tree-structure,
  any node, which has either 'default' either 'type' will be
  considered as a leaf node.

  Defenition syntax: {
    ... nested props
    {
      default: defaultValue,
      type: 'String',
    }
  }

  To set the setting:
    Settings.your.setting.name = value

  To read the setting*:
    Settings.your.setting.name

  * If the setting has default value and localStorage hasn't the property
    the value will be set to localStorage
*/
mciModule.factory('Settings', function(SETTING_DEFS, $log, $window) {
  function isEither(vals) {
    return function(d) { return _.contains(vals, d) }
  }

  function isEndNode(node) {
    return _.isObject(node) && _.any(_.keys(node), isEither(['default', 'type']))
  }

  function buildSettingsTree(obj, contextKey) {
    return _.reduce(_.keys(obj), function(m, key) {
      var v = obj[key]
      if (isEndNode(v)) {
        var accessor = {
          path: contextKey.concat(key).join('.'),
          type: v.type,
          default: v.default,
        }
        m['_' + key] = accessor
        Object.defineProperty(m, key, {
          get: function() {
            return readSetting(this['_' + key])
          },
          set: function(val) {
            writeSetting(this['_' + key], val)
          },
        })
        return m
      } else {
        m[key] = buildSettingsTree(v, contextKey.concat(key))
        return m
      }
    }, {})
  }

  function writeSetting(descriptor, value) {
    localStorage.setItem(descriptor.path, value)
  }

  function readSetting(descriptor) {
    var value = localStorage.getItem(descriptor.path)
    if (value === null) {
      defValue = descriptor.default
      if (defValue !== undefined) {
        value = defValue

        // Restore default
        writeSetting(descriptor, value)
      } else {
        return undefined
      }
    }

    // Type casting
    return $window[descriptor.type || 'String'](value)
  }

  var tree = buildSettingsTree(SETTING_DEFS, [SETTING_DEFS.GLOBAL_PREFIX])
  // For unit testing
  tree._buildSettingsTree = buildSettingsTree

  return tree
})
