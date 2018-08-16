ReactDOM.render(
  <Root
    data={window.serverData}
    project={window.project}
    userTz={window.userTz}
    jiraHost={window.jiraHost}
    user={window.user}
  />, document.getElementById('root')
);
