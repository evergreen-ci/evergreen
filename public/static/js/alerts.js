// TODO we can add other implementations of alert 'classes' here.
// Only email supported for the moment.

function NewEmailAlert(recipient){
  return {
    provider:"email",

    settings:{
      recipient: recipient,
    },

  }
}

