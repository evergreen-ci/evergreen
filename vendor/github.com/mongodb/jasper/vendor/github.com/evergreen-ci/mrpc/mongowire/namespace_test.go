package mongowire

import "testing"

func testNamespaceIsCommand(test *testing.T, ns string, correct bool) {
	if NamespaceIsCommand(ns) != correct {
		test.Errorf("wrong result for %s", ns)
	}
}

func TestNamespaceIsCommand(test *testing.T) {
	testNamespaceIsCommand(test, "foo", false)
	testNamespaceIsCommand(test, "foo.bar", false)
	testNamespaceIsCommand(test, "foo.$cmd", true)
	testNamespaceIsCommand(test, "foo.", false)
}

func testNamespaceToDB(test *testing.T, ns string, db string) {
	s := NamespaceToDB(ns)
	if s != db {
		test.Errorf("NamespaceToDB wrong %s %s %s", ns, db, s)
	}
}

func TestNamespaceToDB(test *testing.T) {
	testNamespaceToDB(test, "foo", "foo")
	testNamespaceToDB(test, "", "")
	testNamespaceToDB(test, "foo.bar", "foo")
	testNamespaceToDB(test, "foo.", "foo")
	testNamespaceToDB(test, "foo.bar.abc", "foo")
}

func testNamespaceToCollection(test *testing.T, ns string, db string) {
	s := NamespaceToCollection(ns)
	if s != db {
		test.Errorf("NamespaceToCollection wrong %s %s %s", ns, db, s)
	}
}

func TestNamespaceToCollection(test *testing.T) {
	testNamespaceToCollection(test, "foo", "")
	testNamespaceToCollection(test, "", "")
	testNamespaceToCollection(test, "foo.bar", "bar")
	testNamespaceToCollection(test, "foo.", "")
	testNamespaceToCollection(test, "foo.bar.abc", "bar.abc")
}
