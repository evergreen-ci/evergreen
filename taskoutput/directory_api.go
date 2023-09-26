package taskoutput

/*
// TODO: versioning?
type DirectoryAPI struct {
	directories map[string]*Directory
}

func (a *DirectoryAPI) AddDirectory(name string) *Directory {
	d, ok := a.directories[name]
	if !ok {
		d = &Directory{
			name:    name,
			methods: map[string]*DirectoryMethod{},
		}
		a.directories[name] = d
	}

	return d
}

func (a *DirectoryAPI) Resolve() error {
	return nil
}

// Start starts creates the directory (and subdirectories) and starts
// "listening" on the registered events.
func (a *DirectoryAPI) Start() error {
	return nil
}

// Close stops the listener thread and kills all routine threads.
func (a *DirectoryAPI) Close() error {
	return nil
}

type Directory struct {
	name    string
	methods map[string]*DirectoryMethod
}

func (d *Directory) AddMethod(name string) *DirectoryMethod {
	m, ok := d.methods[name]
	if !ok {
		m = &DirectoryMethod{name: name}
		d.methods[name] = m
	}

	return m
}

type DirectoryMethod struct {
	name     string
	fsEvents []fsEvent
	handler  Handler
}

func (m *DirectoryMethod) Create(recursive bool) *DirectoryMethod {
	m.fsEvents = append(m.fsEvents, fsEvent{op: watcher.Create, recursive: recursive})

	return m
}

func (m *DirectoryMethod) Write(recursive bool) *DirectoryMethod {
	m.fsEvents = append(m.fsEvents, fsEvent{op: watcher.Write, recursive: recursive})

	return m
}

func (m *DirectoryMethod) Remove(recursive bool) *DirectoryMethod {
	m.fsEvents = append(m.fsEvents, fsEvent{op: watcher.Remove, recursive: recursive})

	return m
}

func (m *DirectoryMethod) Rename(recursive bool) *DirectoryMethod {
	m.fsEvents = append(m.fsEvents, fsEvent{op: watcher.Rename, recursive: recursive})

	return m
}

func (m *DirectoryMethod) Chmod(recursive bool) *DirectoryMethod {
	m.fsEvents = append(m.fsEvents, fsEvent{op: watcher.Chmod, recursive: recursive})

	return m
}

func (m *DirectoryMethod) Move(recursive bool) *DirectoryMethod {
	m.fsEvents = append(m.fsEvents, fsEvent{op: watcher.Move, recursive: recursive})

	return m
}

func (m *DirectoryMethod) SetHandler(h Handler) {
	if m.handler != nil {
		grip.Warningf("called SetHandler more than once for directory method '%s'", m.name)
	}

	m.handler = h
}

type Handler interface {
	Run(ctx context.Context) error
}

type fsEvent struct {
	op        watcher.Op
	recursive bool
}
*/
