mciModule.run(function($templateCache) {
  $templateCache.put('perf-discovery-version-dropdown',
    '<ui-select ng-model="vdvm.model.selected" ' +
               'title="Select version or tag">' +
      '<ui-select-match>' +
        '<span ng-bind="$select.selected.name"></span>' +
      '</ui-select-match>' +
      '<ui-select-choices repeat="item in vdvm.options | filter : $select.search" refresh="vdvm.onRefresh($select.search)" refresh-delay="250">' +
        '<span ng-bind="item.name"></span>' +
      '</ui-select-choices>' +
    '</ui-select>'
  )

  $templateCache.put('perf-discovery-link',
    '<div class="ui-grid-cell-contents">' +
      '<a href="{{grid.appScope.$ctrl.getCellUrl(row, col)}}"' +
         'target="_blank"' +
         'rel="noopener"'+
      '>' +
        '{{COL_FIELD}}' +
      '</a>' +
    '</div>'
  )

  $templateCache.put('perf-discovery-ratio',
    '<div class="ui-grid-cell-contents" style="background: {{color}}">' +
      '{{ratio | percentage | number : 0}}' +
    '</div>'
  )

  $templateCache.put('evg-grid/multiselect-filter',
    '<ui-select ' +
        'multiple ' +
        'append-to-body="true" ' +
        'ng-model="vm.col.filters[0].term" ' +
        'ng-change="vm.triggerFiltering()">' +
      '<ui-select-match placeholder="Choose {{vm.col.name}}">{{$item}}</ui-select-match>' +
      '<ui-select-choices repeat="item in (vm.col.filters[0].options | filter : $select.search)">' +
        '{{item}}' +
      '</ui-select-choices>' +
    '</ui-select>'
  )

  $templateCache.put('evg-ui-select/header-filter',
    '<multiselect-grid-header col="col"></multiselect-grid-header>'
  )
})
