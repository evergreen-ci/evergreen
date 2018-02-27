mciModule.run(function($templateCache) {
  $templateCache.put('evg-grid/multiselect-filter',
    '<ui-select ' +
        'multiple ' +
        'append-to-body="true" ' +
        'ng-model="vm.col.filters[0].term" ' +
        'ng-change="vm.triggerFiltering()">' +
      '<ui-select-match placeholder="Choose {{vm.col.name}}">{{$item}}</ui-select-match>' +
      '<ui-select-choices repeat="item in vm.col.filters[0].options">' +
        '{{item}}' +
      '</ui-select-choices>' +
    '</ui-select>'
  )

  $templateCache.put('evg-ui-select/header-filter',
    '<multiselect-grid-header col="col"></multiselect-grid-header>'
  )
})
