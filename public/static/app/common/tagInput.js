mciModule.component('tagInput', {
  bindings: {
    items: '=',
    klass: '@',
    placeholder: '@',
  },
  templateUrl: '/static/app/common/tagInput.html',
  controller: function($element, $scope) {
    var vm = this

    vm.$postLink = function() {
      var ngModel = $element.find('input').controller('ngModel')

      ngModel.$formatters.push(function(val) {
        return val ? val.join(', ') : ''
      })

      ngModel.$parsers.push(function(val) {
        // Split by comma and trim at the same time
        return val ? _.filter(val.split(/\s*,\s*/)) : []
      })
    }

    return vm
  },
})
