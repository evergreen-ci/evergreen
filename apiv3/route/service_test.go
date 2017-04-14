package route

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/evergreen-ci/evergreen/model/host"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHostPaginator(t *testing.T) {
	numHostsInDB := 300
	Convey("When paginating with a ServiceContext", t, func() {
		serviceContext := servicecontext.MockServiceContext{}
		Convey("and there are hosts to be found", func() {
			cachedHosts := []host.Host{}
			for i := 0; i < numHostsInDB; i++ {
				nextHost := host.Host{
					Id: fmt.Sprintf("host%d", i),
				}
				cachedHosts = append(cachedHosts, nextHost)
			}
			serviceContext.MockHostConnector.CachedHosts = cachedHosts
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				hostToStartAt := 100
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				hostToStartAt := 150
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:    50,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				hostToStartAt := 50
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("host%d", 0),
						Limit:    50,
						Relation: "prev",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
			Convey("then finding a key in the last page should produce only a previous"+
				" page and a limited set of models", func() {
				hostToStartAt := 299
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < numHostsInDB; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Prev: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				hostToStartAt := 0
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id: model.APIString(fmt.Sprintf("host%d", i)),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
				}
				checkPaginatorResultMatches(hostPaginator, fmt.Sprintf("host%d", hostToStartAt),
					limit, &serviceContext, expectedPages, expectedHosts, nil)

			})
		})
	})
}

func checkPaginatorResultMatches(paginator PaginatorFunc, key string, limit int,
	sc servicecontext.ServiceContext, expectedPages *PageResult,
	expectedModels []model.Model, expectedErr error) {
	res, pages, err := paginator(key, limit, sc)
	So(err, ShouldEqual, expectedErr)
	So(res, ShouldResemble, expectedModels)
	So(pages, ShouldResemble, expectedPages)
}
