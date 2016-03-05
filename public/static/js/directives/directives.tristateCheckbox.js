var directives = directives || {};

directives.tristateCheckbox = angular.module('directives.tristateCheckbox', []);

directives.tristateCheckbox.directive('tristateCheckbox', function(){
  return {
    require: '?ngModel',
    link: function(scope, el, attrs, ctrl) {
      // Tri-state checkboxes can be bound to an ng-model (like a regular 
      // checkbox) but instead of rendering according to just true/false,
      // it renders according to 3 possible values for the model it's bound to
      var truthy = true; // checked
      var falsy = false; // unchecked
      var nully = null;  // indeterminate!

      ctrl.$formatters = []; 
      ctrl.$parsers = [];

      ctrl.$render = function() {
        var d = ctrl.$viewValue; // gets the value bound via ng-model

        // el is the actual DOM element bound to this instance of 
        // the directive - set its checked/indeterminate DOM properties
        // according to the value in the model
        el.data('checked', d);
        switch(d){
          case truthy:
            el.prop('indeterminate', false);
            el.prop('checked', true);
            break;
          case falsy:
            el.prop('indeterminate', false);
            el.prop('checked', false);
            break;
          default:
            el.prop('indeterminate', true);
        }
      };

      // Override the behavior for the click handler for the checkbox.
      // This is the value that will actually get sent to the getter/setter 
      // function for ng-model, when that option is in effect.
      el.bind('click', function() {
        var d;
        switch(el.data('checked')){
          case falsy:
            d = truthy;
            break;
          case truthy:
            d = nully;
            break;
          default:
            d = falsy;
        }
        ctrl.$setViewValue(d);
        scope.$apply(ctrl.$render);
      });
    }
  };
})
