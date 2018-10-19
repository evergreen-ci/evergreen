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
