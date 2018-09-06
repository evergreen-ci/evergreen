mciModule.run(function($templateCache) {
  $templateCache.put('hazard-level-cell',
    '<div>' +
      '<div class="ui-grid-cell-contents" style="text-align: center;">' +
        '<div ng-if="row.groupHeader" style="font-weight: bold;" ng-style="{color: level.color}">' +
          '{{level.label}} {{cnt}} {{sv | number:2}}' +
        '</div>' +
        '<div ng-if="!row.groupHeader" ng-style="{color: color}">' +
          '{{ratio > 0 ? "+" : ""}}{{ratio | number : 0}}%' +
        '</div>' +
      '</div>' +
    '</div>'
  )
})
