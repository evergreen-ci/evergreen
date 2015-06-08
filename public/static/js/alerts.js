// We can add other implementations of alert 'classes' here.
// Only email supported for the moment. See EVG-43 for flowdock support.

function NewEmailAlert(recipient){
  return {
    provider:"email",

    settings:{
      recipient: recipient,
    },

  }
}

