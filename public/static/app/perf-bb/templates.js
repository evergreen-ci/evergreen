mciModule.run(function($templateCache) {
  $templateCache.put('hazard-level-cell',
    '<div>' +
      '<div class="ui-grid-cell-contents" style="text-align: center;">' +
        '<div ng-if="row.groupHeader" >' +
          '<span style="font-weight: bold;" ng-style="{color: level.color}">{{level.label}} </span>' +
          '({{count}})' +
        '</div>' +
        '<div ng-if="!row.groupHeader" ng-style="{color: color}">' +
          '{{ratio > 0 ? "+" : ""}}{{ratio | number : 0}}%' +
        '</div>' +
      '</div>' +
    '</div>'
  )
})
