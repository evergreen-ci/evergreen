package jsonx

func errOrPanic(err error) {
	if err != nil {
		panic(err)
	}
}

func docConstructorOrPanic(doc *Document, err error) *Document { errOrPanic(err); return doc }
func arrayConstructorOrPanic(a *Array, err error) *Array       { errOrPanic(err); return a }
func valueConstructorOrPanic(v *Value, err error) *Value       { errOrPanic(err); return v }
