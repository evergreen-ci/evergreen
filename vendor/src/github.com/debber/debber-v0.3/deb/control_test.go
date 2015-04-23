package deb

import (
	"testing"
)

func TestNewControl(t *testing.T) {
	c1 := NewControlDefault("testpkg", "me", "me@a.c", "does nothing", "testpkg is a dummy package. It does nothing", true)
	if len(*c1) != 3 {
		t.Errorf("Should be 3 paragraphs! %d", len(*c1))
	}
}

func TestGetParasByField(t *testing.T) {
	c1 := NewControlDefault("testpkg", "me", "me@a.c", "does nothing", "testpkg is a dummy package. It does nothing", true)
	ss := c1.GetParasByField(SourceFName, "testpkg")
	if len(ss) != 1 {
		t.Errorf("Should be 1 source paragraphs! %d", len(ss))
	}
	ps := c1.GetParasByField(PackageFName, "testpkg")
	if len(ps) != 1 {
		t.Errorf("Should be 1 bin paragraphs! %d", len(ps))
	}
	c2 := Control(ss) //, ps...}
	c2 = append(c2, ps...)
	if len(c2) != 2 {
		t.Errorf("Should be 2 paragraphs! %d", len(c2))
	}
}
