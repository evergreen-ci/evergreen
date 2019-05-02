mciModule.run(function($templateCache) {
  $templateCache.put('ui-grid-group-name',
    '<div>' +
      '<div '+
        'ng-if="'+
          '!col.grouping || ' +
          'col.grouping.groupPriority === undefined || ' + 
          'col.grouping.groupPriority === null || ' +
          '( row.groupHeader && col.grouping.groupPriority === row.treeLevel )" ' +
        'class="ui-grid-cell-contents" ' +
        'title="TOOLTIP">{{COL_FIELD CUSTOM_FILTERS}}' +
      '</div>' +
    '</div>'
  )

  $templateCache.put('ui-grid-link',
    '<div class="ui-grid-cell-contents">' +
      '<a ng-href="{{col.colDef._link(row, col)}}" target="_blank" rel="noopener">{{COL_FIELD}}</a>' +
    '</div>'
  )
})
