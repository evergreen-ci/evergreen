// We can add other implementations of alert 'classes' here.
// Only email supported for the moment. See EVG-43 for flowdock support.

function NewAlert(recipient){
  // JIRA alerts are denoted with "JIRA:project:issue type" format
  if (recipient.startsWith("JIRA:")) {
    var args = recipient.split(":")
    return {
      provider: "jira",
      settings: {
        project: args[1],
        issue: args[2],
      },
    }
  }
  // otherwise always default to email
  return {
    provider: "email",
    settings: {
      recipient: recipient,
    },

  }
}

