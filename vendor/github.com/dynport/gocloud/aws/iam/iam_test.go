package iam

import (
	"encoding/xml"
	"io/ioutil"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func mustReadFixture(t *testing.T, name string) []byte {
	b, e := ioutil.ReadFile("fixtures/" + name)
	if e != nil {
		t.Fatal("fixture " + name + " does not exist")
	}
	return b
}

func TestIam(t *testing.T) {
	Convey("IAM", t, func() {
		Convey("GetUser", func() {
			f := mustReadFixture(t, "get_user.xml")
			rsp := &GetUserResponse{}
			e := xml.Unmarshal(f, rsp)
			So(e, ShouldBeNil)
			So(f, ShouldNotBeNil)
			user := rsp.User
			So(user.Path, ShouldEqual, "/division_abc/subdivision_xyz/")
			So(user.UserName, ShouldEqual, "Bob")
			So(strings.TrimSpace(user.Arn), ShouldEqual, "arn:aws:iam::123456789012:user/division_abc/subdivision_xyz/Bob")
		})

		Convey("AccountSummary", func() {
			f := mustReadFixture(t, "get_account_summary.xml")
			rsp := &GetAccountSummaryResponse{}
			e := xml.Unmarshal(f, rsp)
			So(e, ShouldBeNil)
			So(f, ShouldNotBeNil)
			m := rsp.SummaryMap
			So(len(m.Entries), ShouldEqual, 14)

			entry := m.Entries[0]
			So(entry.Key, ShouldEqual, "Groups")
			So(entry.Value, ShouldEqual, "31")

		})

	})

	Convey("ListUsers", t, func() {
		f := mustReadFixture(t, "list_users.xml")
		rsp := &ListUsersResponse{}
		e := xml.Unmarshal(f, rsp)
		So(e, ShouldBeNil)
		So(len(rsp.Users), ShouldEqual, 2)
		So(rsp.Users[0].UserName, ShouldEqual, "Andrew")

	})

	Convey("ListAccountAliases", t, func() {
		f := mustReadFixture(t, "list_account_aliases.xml")
		rsp := &ListAccountAliasesResponse{}
		e := xml.Unmarshal(f, rsp)
		So(e, ShouldBeNil)
		So(len(rsp.AccountAliases), ShouldEqual, 1)
		So(rsp.AccountAliases[0], ShouldEqual, "foocorporation")

	})
}
