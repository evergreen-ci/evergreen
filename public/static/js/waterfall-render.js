ReactDOM.render(
  React.createElement(Root, {data: window.serverData, project: window.project, userTz: window.userTz}),
  document.getElementById('root')
);
