package main

type dropletDelete struct {
	Id string `cli:"arg required"`
}

func (r *dropletDelete) Run() error {
	cl, e := client()
	if e != nil {
		return e
	}
	return cl.DropletDelete(r.Id)
}
