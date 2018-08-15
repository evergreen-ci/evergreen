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

})
