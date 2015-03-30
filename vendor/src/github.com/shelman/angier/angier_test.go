package angier

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func TestTransferByFieldNames(t *testing.T) {

	Convey("With two structs of the same type", t, func() {

		type s struct {
			A int
			B string
			C bool
			D time.Time

			// not exported, shouldn't be copied
			e bool
		}

		Convey("transferring from one to the other should copy over all of"+
			" the exported fields", func() {

			src := &s{
				A: 2,
				B: "hi",
				C: true,
				D: time.Now(),
				e: true,
			}

			dest := &s{}

			So(TransferByFieldNames(src, dest), ShouldBeNil)
			So(dest, ShouldResemble,
				&s{A: src.A, B: src.B, C: src.C, D: src.D, e: false})

		})

	})

	Convey("With two structs of different types", t, func() {

		type t1 struct {
			Field string
		}

		type t2 struct {
			Field string
		}

		type s1 struct {
			// two matching fields
			MatchOne int
			MatchTwo bool

			// two fields that don't appear in the other struct
			OnlySrcOne int
			OnlySrcTwo string

			// two fields with the same name but the wrong kind
			WrongKindOne int
			WrongKindTwo bool

			// one field with the same name and kind but the wrong type
			WrongType t1
		}

		type s2 struct {
			// two matching fields
			MatchOne int
			MatchTwo bool

			// two fields that don't appear in the other struct
			OnlyDestOne int
			OnlyDestTwo string

			// two fields with the same name but the wrong kind
			WrongKindOne bool
			WrongKindTwo int

			// one field with the same name and kind but the wrong type
			WrongType t2
		}

		Convey("transferring from one to the other should copy only the"+
			" fields with the same name and type, and leave the other fields"+
			" untouched", func() {

			src := &s1{
				MatchOne:     2,
				MatchTwo:     true,
				OnlySrcOne:   1,
				OnlySrcTwo:   "hi",
				WrongKindOne: 3,
				WrongKindTwo: true,
				WrongType:    t1{Field: "yo"},
			}

			dest := &s2{}

			So(TransferByFieldNames(src, dest), ShouldBeNil)
			So(dest, ShouldResemble, &s2{MatchOne: 2, MatchTwo: true})

		})

	})
}

func TestTransferByTags(t *testing.T) {

	Convey("With two structs of the same type", t, func() {

		type s struct {
			A int  `angier:"a"`
			B bool `angier:"b"`
			C string
			d int `angier:"d"`
		}

		Convey("all exported and tagged fields should be transferred", func() {

			src := &s{
				A: 2,
				B: true,
				C: "hi",
				d: 3,
			}

			dest := &s{}

			So(TransferByTags(src, dest), ShouldBeNil)
			So(dest, ShouldResemble, &s{A: src.A, B: src.B})

		})

	})

	Convey("With two structs of different types", t, func() {

		Convey("fields with no struct tag in the source struct should be"+
			" ignored", func() {

			type s1 struct {
				A int
			}

			type s2 struct {
				A int `angier:"a"`
			}

			src := &s1{
				A: 2,
			}
			dest := &s2{}

			So(TransferByTags(src, dest), ShouldBeNil)
			So(dest, ShouldResemble, &s2{})

		})

		Convey("fields with no struct tag in the destination struct should be"+
			" ignored", func() {

			type s1 struct {
				A int `angier:"a"`
			}

			type s2 struct {
				A int
			}

			src := &s1{
				A: 2,
			}
			dest := &s2{}

			So(TransferByTags(src, dest), ShouldBeNil)
			So(dest, ShouldResemble, &s2{})

		})

		Convey("fields with the same name but different tags should be"+
			" ignored", func() {

			type s1 struct {
				A int `angier:"a"`
			}

			type s2 struct {
				A int `angier:"b"`
			}

			src := &s1{
				A: 2,
			}
			dest := &s2{}

			So(TransferByTags(src, dest), ShouldBeNil)
			So(dest, ShouldResemble, &s2{})

		})

		Convey("fields with the same struct tag but different types should"+
			" be ignored", func() {

			type s1 struct {
				A int `angier:"a"`
			}

			type s2 struct {
				A bool `angier:"b"`
			}

			src := &s1{
				A: 2,
			}
			dest := &s2{}

			So(TransferByTags(src, dest), ShouldBeNil)
			So(dest, ShouldResemble, &s2{})

		})

		Convey("unexported fields should be ignored", func() {

			type s1 struct {
				a int `angier:"a"`
			}

			type s2 struct {
				a int `angier:"a"`
			}

			src := &s1{
				a: 2,
			}
			dest := &s2{}

			So(TransferByTags(src, dest), ShouldBeNil)
			So(dest, ShouldResemble, &s2{})

		})

		Convey("exported fields of the same type, with the same struct tags,"+
			" should be transferred", func() {

			type s1 struct {
				A  int       `angier:"a"`
				B1 bool      `angier:"b"`
				C  time.Time `angier:"c"`
			}

			type s2 struct {
				A  int       `angier:"a"`
				B2 bool      `angier:"b"`
				C  time.Time `angier:"c"`
			}

			src := &s1{
				A:  2,
				B1: true,
				C:  time.Now(),
			}
			dest := &s2{}

			So(TransferByTags(src, dest), ShouldBeNil)
			So(dest, ShouldResemble, &s2{A: src.A, B2: src.B1, C: src.C})

		})

	})

}
