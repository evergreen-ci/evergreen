mciModule.run(function($templateCache) {
  $templateCache.put('hazard-level-cell',
    '<div>' +
      '<div class="ui-grid-cell-contents" style="text-align: center;">' +
        '<div ng-if="row.groupHeader" >' +
          '<micro-hazard-chart points="points" ctx="ctx" />' +
        '</div>' +
        '<div ng-if="!row.groupHeader" ng-style="{color: color}">' +
          '{{ratio > 0 ? "+" : ""}}{{ratio | number : 0}}%' +
        '</div>' +
      '</div>' +
    '</div>'
  )

  $templateCache.put('grouped-link-cell',
    '<div>' +
      '<div class="ui-grid-cell-contents">' +
        '<div ng-if="row.groupHeader" >' +
          '<strong>{{COL_FIELD}}</strong>' +
        '</div>' +
        '<div ng-if="!row.groupHeader">' +
          '<a ng-href="{{col.colDef._link(row, col)}}" target="_blank" rel="noopener">{{COL_FIELD}}</a>' +
        '</div>' +
      '</div>' +
    '</div>'
  )
})
