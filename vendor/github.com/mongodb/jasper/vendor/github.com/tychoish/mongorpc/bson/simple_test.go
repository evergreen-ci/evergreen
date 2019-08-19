package bson

import (
	"testing"

	"gopkg.in/mgo.v2/bson"
)

func TestBSONIndexOf(test *testing.T) {
	doc := bson.D{{Name: "a", Value: 1}, {Name: "b", Value: 3}}

	if 0 != bsonIndexOf(doc, "a") {
		test.Errorf("index of a is wrong")
	}

	if 1 != bsonIndexOf(doc, "b") {
		test.Errorf("index of b is wrong")
	}

	if -1 != bsonIndexOf(doc, "c") {
		test.Errorf("index of c is wrong")
	}
}

type testWalker struct {
	seen []bson.DocElem
}

func (tw *testWalker) Visit(elem *bson.DocElem) error {
	tw.seen = append(tw.seen, *elem)
	if elem.Value.(int) == 111 {
		return walkAbortSignal
	}
	elem.Value = 17
	return nil
}

func TestBSONWalk1(test *testing.T) {
	doc := bson.D{{Name: "a", Value: 1}, {Name: "b", Value: 3}}
	walker := &testWalker{}
	doc, err := BSONWalk(doc, "b", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
	}
	if len(walker.seen) != 1 {
		test.Errorf("wrong # saw")
	}
	if walker.seen[0].Name != "b" {
		test.Errorf("name wrong %s", walker.seen[0].Name)
	}
	if doc[1].Value.(int) != 17 {
		test.Errorf("we didn't change it %d", doc[1].Value.(int))
	}
}

func TestBSONWalk2(test *testing.T) {
	doc := bson.D{{"a", 1}, {"b", 3}, {"c", bson.D{{"x", 5}, {"y", 7}}}}
	walker := &testWalker{}
	doc, err := BSONWalk(doc, "c.y", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
		return
	}
	if len(walker.seen) != 1 {
		test.Errorf("wrong # saw")
		return
	}
	if walker.seen[0].Value != 7 {
		test.Errorf("number wrong %d", walker.seen[0].Value)
	}
	if doc[2].Value.(bson.D)[1].Value.(int) != 17 {
		test.Errorf("we didn't change it")
	}
}

func TestBSONWalk3(test *testing.T) {
	doc := bson.D{{"a", 1}, {"b", 3}, {"c", []bson.D{bson.D{{"x", 5}}, bson.D{{"x", 7}}}}}
	walker := &testWalker{}
	_, err := BSONWalk(doc, "c.x", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
		return
	}
	if len(walker.seen) != 2 {
		test.Errorf("wrong # saw %d", len(walker.seen))
		return
	}
	if walker.seen[0].Value != 5 {
		test.Errorf("number wrong %d", walker.seen[0].Value)
	}
	if walker.seen[1].Value != 7 {
		test.Errorf("number wrong %d", walker.seen[1].Value)
	}
}

func TestBSONWalk4(test *testing.T) {
	doc := bson.D{{"a", 1}, {"b", 3}, {"c", []interface{}{bson.D{{"x", 5}}, bson.D{{"x", 7}}}}}
	walker := &testWalker{}
	_, err := BSONWalk(doc, "c.x", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
		return
	}
	if len(walker.seen) != 2 {
		test.Errorf("wrong # saw %d", len(walker.seen))
		return
	}
	if walker.seen[0].Value != 5 {
		test.Errorf("number wrong %d", walker.seen[0].Value)
	}
	if walker.seen[1].Value != 7 {
		test.Errorf("number wrong %d", walker.seen[1].Value)
	}
}

func TestBSONWalk5(test *testing.T) {
	doc := bson.D{{"a", 1}, {"b", 3}, {"c", []interface{}{bson.D{{"x", 5}}, bson.D{{"x", 111}, {"y", 3}}, bson.D{{"x", 7}}}}}
	walker := &testWalker{}
	doc, err := BSONWalk(doc, "c.x", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
		return
	}
	if len(walker.seen) != 3 {
		test.Errorf("wrong # saw %d", len(walker.seen))
		return
	}
	if walker.seen[0].Value != 5 {
		test.Errorf("number wrong %d", walker.seen[0].Value)
		return
	}
	if walker.seen[2].Value != 7 {
		test.Errorf("number wrong %d", walker.seen[1].Value)
		return
	}
	if len(doc[2].Value.([]interface{})) != 2 {
		test.Errorf("did not remove %s", doc[2])
		return
	}

}

func TestBSONWalk6(test *testing.T) {
	doc := bson.D{{"a", 1}, {"b", 3}, {"c", []bson.D{bson.D{{"x", 5}}, bson.D{{"x", 111}, {"y", 3}}, bson.D{{"x", 7}}}}}
	walker := &testWalker{}
	doc, err := BSONWalk(doc, "c.x", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
		return
	}
	if len(walker.seen) != 3 {
		test.Errorf("wrong # saw %d", len(walker.seen))
		return
	}
	if walker.seen[0].Value != 5 {
		test.Errorf("number wrong %d", walker.seen[0].Value)
		return
	}
	if walker.seen[2].Value != 7 {
		test.Errorf("number wrong %d", walker.seen[1].Value)
		return
	}
	if len(doc[2].Value.([]bson.D)) != 2 {
		test.Errorf("did not remove %s", doc[2])
		return
	}

}

func TestBSONWalk7(test *testing.T) {
	doc := bson.D{{"a", 111}, {"b", 3}}
	walker := &testWalker{}
	doc, err := BSONWalk(doc, "a", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
	}
	if len(doc) != 1 {
		test.Errorf("didn't delete 1 %s", doc)
	}
	if doc[0].Name != "b" {
		test.Errorf("deleted wrong one? %s", doc)
	}

}

func TestBSONWalk8(test *testing.T) {
	doc := bson.D{{"b", 11}, {"a", 111}}
	walker := &testWalker{}
	doc, err := BSONWalk(doc, "a", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
	}
	if len(doc) != 1 {
		test.Errorf("didn't delete 1 %s", doc)
	}
	if doc[0].Name != "b" {
		test.Errorf("deleted wrong one? %s", doc)
	}

}

func TestBSONWalk9(test *testing.T) {
	doc := bson.D{{"b", 11}, {"a", 111}, {"c", 12}}
	walker := &testWalker{}
	doc, err := BSONWalk(doc, "a", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
	}
	if len(doc) != 2 {
		test.Errorf("didn't delete 1 %s", doc)
	}
	if doc[0].Name != "b" {
		test.Errorf("deleted wrong one? %s", doc)
	}
	if doc[1].Name != "c" {
		test.Errorf("deleted wrong one? %s", doc)
	}

}

func TestBSONWalk10(test *testing.T) {
	doc := bson.D{{"b", 11}, {"a", bson.D{{"x", 1}, {"y", 111}}}, {"c", 12}}
	walker := &testWalker{}
	doc, err := BSONWalk(doc, "a.y", walker.Visit)
	if err != nil {
		test.Errorf("why did we get an error %s", err)
	}
	if len(doc) != 3 {
		test.Errorf("what did i do! %s", doc)
	}

	if doc[1].Name != "a" {
		test.Errorf("what did i do! %s", doc)
	}

	sub := doc[1].Value.(bson.D)
	if len(sub) != 1 {
		test.Errorf("didn't delete %s", doc)
	}
	if sub[0].Name != "x" {
		test.Errorf("deleted wrong one? %s", doc)
	}

}
