function waterfallRef(ref) {
  window.waterfallInstance = ref;
}

ReactDOM.render(
  <Root
    ref={waterfallRef}
    data={window.serverData}
    project={window.project}
    userTz={window.userTz}
    jiraHost={window.jiraHost}
    user={window.user}
  />, document.getElementById('root')
);
