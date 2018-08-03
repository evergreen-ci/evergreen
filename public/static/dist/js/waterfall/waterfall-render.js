'use strict';

function waterfallRef(ref) {
  window.waterfallInstance = ref;
}

ReactDOM.render(React.createElement(Root, {
  ref: waterfallRef,
  data: window.serverData,
  project: window.project,
  userTz: window.userTz,
  jiraHost: window.jiraHost,
  user: window.user
}), document.getElementById('root'));
//# sourceMappingURL=waterfall-render.js.map
