db.users.Insert(
    {
        _id: "github_pull_request"
        first_name: "Github"
        last_name : "Pull Request"
        display_name: "Github Pull Request"
        email: "todo@mongodb.com", // TODO
        created_at: new Date(),
        settings: {},
        apikey: (Math.random() + 1).toString(128),
    }
)
