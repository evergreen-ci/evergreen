var addSelectorsAndOwnerType = function(subscription, type, id) {
  if (!subscription) {
    return;
  }
  if (!subscription.selectors) {
    subscription.selectors = [];
  }
  subscription.selectors.push({
    Type: "object",
    Data: type
  });
  subscription.selectors.push({
    Type: "id",
    Data: id
  });
  subscription.owner_type = "person";
};
