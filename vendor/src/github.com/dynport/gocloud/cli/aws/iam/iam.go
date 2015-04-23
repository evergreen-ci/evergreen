package iam

import (
	"fmt"
	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/aws/iam"
	"strings"
)

func Register(router *cli.Router) {
	router.RegisterFunc("aws/iam/users/get", iamGetUser, "Get user information")
	router.RegisterFunc("aws/iam/users/list", iamListUsers, "List users")
	router.RegisterFunc("aws/iam/account-summary", iamGetAccountSummary, "Get account summary")
	router.RegisterFunc("aws/iam/account-aliases/list", iamListAccountAliases, "List account aliases")
}

func iamGetUser() error {
	client := iam.NewFromEnv()
	user, e := client.GetUser("")
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	table.Add("Id", user.UserId)
	table.Add("Name", user.UserName)
	table.Add("Arn", strings.TrimSpace(user.Arn))
	table.Add("Path", user.Path)
	fmt.Println(table)
	return nil
}

func iamGetAccountSummary() error {
	client := iam.NewFromEnv()
	summary, e := client.GetAccountSummary()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	for _, entry := range summary.Entries {
		table.Add(entry.Key, entry.Value)
	}
	fmt.Println(table)
	return nil
}

func iamListUsers() error {
	client := iam.NewFromEnv()
	rsp, e := client.ListUsers()
	if e != nil {
		return e
	}
	table := gocli.NewTable()
	for _, user := range rsp.Users {
		table.Add(user.UserId, user.UserName, strings.TrimSpace(user.Arn))
	}
	fmt.Println(table)
	return nil
}

func iamListAccountAliases() error {
	client := iam.NewFromEnv()
	rsp, e := client.ListAccountAliases()
	if e != nil {
		return e
	}
	for _, alias := range rsp.AccountAliases {
		fmt.Println(alias)
	}
	return nil
}
