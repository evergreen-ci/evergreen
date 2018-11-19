mciModule.directive('multiselectGridHeader', function(
  uiGridConstants
) { return {
  restrict: 'E',
  scope: {
    col: '=',
  },
  bindToController: true,
  controllerAs: 'vm',
  templateUrl: 'evg-grid/multiselect-filter',
  controller: function($scope) {
    var vm = this

    vm.triggerFiltering = function() {
      vm.col.grid.notifyDataChange(uiGridConstants.dataChange.COLUMN)
    }

    function createMatcher(query) {
      if (query == '') return _.constant(true)
      var re = new RegExp(query, 'i')
      return function(item) {
        return re.test(item)
      }
    }

    $scope.$watch('vm.$select.search', function(query) {
      vm.matcher = createMatcher(query)
    })

    return vm
  }
}})
