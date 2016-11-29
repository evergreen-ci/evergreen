// We can add other implementations of alert 'classes' here.
// Only email supported for the moment. See EVG-43 for flowdock support.

function NewAlert(recipient){
  // JIRA alerts are denoted with "JIRA:project" format
  if (recipient.startsWith("JIRA:")) {
    return {
      provider: "jira",
      settings: {
        project: recipient.slice(5),
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

