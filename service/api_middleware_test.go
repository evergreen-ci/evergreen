package service

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckHostWrapper(t *testing.T) {
	h1 := host.Host{
		Id:          "h1",
		Secret:      "swordfish",
		RunningTask: "t1",
	}
	t1 := task.Task{
		Id:     "t1",
		Secret: "password",
		HostId: "h1",
	}

	Convey("With a simple checkTask and checkHost-wrapped route", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		var ctx map[interface{}]interface{}
		as, err := NewAPIServer(testutil.TestConfig(), nil)
		if err != nil {
			t.Fatalf("creating test API server: %v", err)
		}
		root := mux.NewRouter()
		root.HandleFunc("/{taskId}/", as.checkTask(true, as.checkHost(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx = context.GetAll(r)
				as.WriteJSON(w, http.StatusOK, nil)
			}),
		)))

		Convey("and documents representing a proper host-task relationship", func() {
			So(t1.Insert(), ShouldBeNil)
			So(h1.Insert(), ShouldBeNil)

			w := httptest.NewRecorder()
			r, err := http.NewRequest("GET", "/t1/", nil)
			if err != nil {
				t.Fatalf("building request: %v", err)
			}

			Convey("a request without proper task fields should fail", func() {
				root.ServeHTTP(w, r)
				So(w.Code, ShouldEqual, http.StatusConflict)

				Convey("and attach nothing to the context", func() {
					So(ctx[apiTaskKey], ShouldBeNil)
					So(ctx[apiHostKey], ShouldBeNil)
				})
			})
			Convey("a request with proper task fields but no host fields should pass", func() {
				r.Header.Add(evergreen.TaskSecretHeader, t1.Secret)
				root.ServeHTTP(w, r)
				So(w.Code, ShouldEqual, http.StatusBadRequest)

				Convey("and attach nothing to the context", func() {
					So(ctx[apiTaskKey], ShouldBeNil)
					So(ctx[apiHostKey], ShouldBeNil)
				})
			})
			Convey("a request with proper task fields and host fields should pass", func() {
				r.Header.Add(evergreen.TaskSecretHeader, t1.Secret)
				r.Header.Add(evergreen.HostHeader, h1.Id)
				r.Header.Add(evergreen.HostSecretHeader, h1.Secret)
				root.ServeHTTP(w, r)
				So(w.Code, ShouldEqual, http.StatusOK)

				Convey("and attach the and host to the context", func() {
					So(ctx[apiTaskKey], ShouldNotBeNil)
					So(ctx[apiHostKey], ShouldNotBeNil)
					asHost, ok := ctx[apiHostKey].(*host.Host)
					So(ok, ShouldBeTrue)
					So(asHost.Id, ShouldEqual, h1.Id)
					Convey("with an updated LastCommunicationTime", func() {
						So(asHost.LastCommunicationTime, ShouldHappenWithin, time.Second, time.Now())
					})
				})
			})
			Convey("a request with the wrong host secret should fail", func() {
				r.Header.Add(evergreen.TaskSecretHeader, t1.Secret)
				r.Header.Add(evergreen.HostHeader, h1.Id)
				r.Header.Add(evergreen.HostSecretHeader, "bad thing!!!")
				root.ServeHTTP(w, r)
				So(w.Code, ShouldEqual, http.StatusConflict)
				msg, _ := ioutil.ReadAll(w.Body)
				So(string(msg), ShouldContainSubstring, "secret")

				Convey("and attach nothing to the context", func() {
					So(ctx[apiTaskKey], ShouldBeNil)
					So(ctx[apiHostKey], ShouldBeNil)
				})
			})
		})
		Convey("and documents representing a mismatched host-task relationship", func() {
			h2 := host.Host{
				Id:          "h2",
				Secret:      "swordfish",
				RunningTask: "t29",
			}
			t2 := task.Task{
				Id:     "t2",
				Secret: "password",
				HostId: "h50",
			}
			So(t2.Insert(), ShouldBeNil)
			So(h2.Insert(), ShouldBeNil)

			w := httptest.NewRecorder()
			r, err := http.NewRequest("GET", "/t2/", nil)
			if err != nil {
				t.Fatalf("building request: %v", err)
			}

			Convey("a request with proper task fields and host fields should fail", func() {
				r.Header.Add(evergreen.TaskSecretHeader, t2.Secret)
				r.Header.Add(evergreen.HostHeader, h2.Id)
				r.Header.Add(evergreen.HostSecretHeader, h2.Secret)
				root.ServeHTTP(w, r)
				So(w.Code, ShouldEqual, http.StatusConflict)
				msg, _ := ioutil.ReadAll(w.Body)
				So(string(msg), ShouldContainSubstring, "should be running")

				Convey("and attach the and host to the context", func() {
					So(ctx[apiTaskKey], ShouldBeNil)
					So(ctx[apiHostKey], ShouldBeNil)
				})
			})
		})
	})
	Convey("With a checkTask and checkHost-wrapped route using URL params", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		var ctx map[interface{}]interface{}
		as, err := NewAPIServer(testutil.TestConfig(), nil)
		if err != nil {
			t.Fatalf("creating test API server: %v", err)
		}
		root := mux.NewRouter()
		root.HandleFunc("/{taskId}/{hostId}", as.checkTask(true, as.checkHost(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx = context.GetAll(r)
				as.WriteJSON(w, http.StatusOK, nil)
			}),
		)))

		Convey("and documents representing a proper host-task relationship", func() {

			So(t1.Insert(), ShouldBeNil)
			So(h1.Insert(), ShouldBeNil)

			w := httptest.NewRecorder()
			r, err := http.NewRequest("GET", "/t1/h1", nil)
			if err != nil {
				t.Fatalf("building request: %v", err)
			}

			Convey("a request with proper host params and fields should pass", func() {
				r.Header.Add(evergreen.TaskSecretHeader, t1.Secret)
				r.Header.Add(evergreen.HostSecretHeader, h1.Secret)
				root.ServeHTTP(w, r)
				So(w.Code, ShouldEqual, http.StatusOK)

				Convey("and attach the and host to the context", func() {
					So(ctx[apiTaskKey], ShouldNotBeNil)
					So(ctx[apiHostKey], ShouldNotBeNil)
					asHost, ok := ctx[apiHostKey].(*host.Host)
					So(ok, ShouldBeTrue)
					So(asHost.Id, ShouldEqual, h1.Id)
					Convey("with an updated LastCommunicationTime", func() {
						So(asHost.LastCommunicationTime, ShouldHappenWithin, time.Second, time.Now())
					})
				})
			})
			Convey("a request with the wrong host secret should fail", func() {
				r.Header.Add(evergreen.TaskSecretHeader, t1.Secret)
				r.Header.Add(evergreen.HostSecretHeader, "bad thing!!!")
				root.ServeHTTP(w, r)
				So(w.Code, ShouldEqual, http.StatusConflict)
				msg, _ := ioutil.ReadAll(w.Body)
				So(string(msg), ShouldContainSubstring, "secret")

				Convey("and attach nothing to the context", func() {
					So(ctx[apiTaskKey], ShouldBeNil)
					So(ctx[apiHostKey], ShouldBeNil)
				})
			})
		})
	})
}
