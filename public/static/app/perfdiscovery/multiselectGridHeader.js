mciModule.directive('multiselectGridHeader', function(uiGridConstants) { return {
  restrict: 'E',
  scope: {
    col: '=',
  },
  bindToController: true,
  controllerAs: 'vm',
  templateUrl: 'evg-grid/multiselect-filter',
  controller: function() {
    var vm = this

    vm.triggerFiltering = function() {
      vm.col.grid.notifyDataChange(uiGridConstants.dataChange.COLUMN)
    }

    return vm
  }
}})
