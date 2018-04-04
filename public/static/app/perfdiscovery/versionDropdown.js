mciModule.directive('versionDropdown', function(PerfDiscoveryDataService) { return {
  restrict: 'E',
  scope: {
    model: '=',
  },
  bindToController: true,
  controllerAs: 'vdvm',
  templateUrl: 'perf-discovery-version-dropdown',
  controller: function() {
    // Version Dropdown View Model
    var vdvm = this
    vdvm.options = []

    vdvm.getVersionOptions = PerfDiscoveryDataService.getVersionOptions

    vdvm.onRefresh = function(query) {
      // TODO redesign this a bit - part of this function should be moved here
      vdvm.options = vdvm.getVersionOptions(vdvm.model.options, query)
    }

    return vdvm
  }
}})
