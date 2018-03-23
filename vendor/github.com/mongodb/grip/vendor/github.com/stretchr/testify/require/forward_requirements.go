package require

// Assertions provides assertion methods around the
// TestingT interface.
type Assertions struct {
	t TestingT
}

// New makes a new Assertions object for the specified TestingT.
func New(t TestingT) *Assertions {
	return &Assertions{
		t: t,
	}
}

//go:generate go run ../_codegen/main.go -output-package=require -template=require_forward.go.tmpl -include-format-funcs
