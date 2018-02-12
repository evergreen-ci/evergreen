package patch

import "sync"

var intentFactoryRegistry *patchIntentFactoryRegistry

type patchIntentFactory func() Intent
type patchIntentFactoryRegistry struct {
	r map[string]patchIntentFactory
	m sync.RWMutex
}

func (r *patchIntentFactoryRegistry) get(intentType string) (patchIntentFactory, bool) {
	r.m.RLock()
	defer r.m.RUnlock()

	i, ok := r.r[intentType]
	return i, ok
}

// GetIntent returns a concrete Intent object of the type specified by intentType.
func GetIntent(intentType string) (Intent, bool) {
	factory, ok := intentFactoryRegistry.get(intentType)
	if !ok {
		return nil, false
	}

	return factory(), true
}

func init() {
	intentFactoryRegistry = &patchIntentFactoryRegistry{
		r: map[string]patchIntentFactory{
			GithubIntentType: func() Intent { return &githubIntent{} },
			CliIntentType:    func() Intent { return &cliIntent{} },
		},
	}
}
