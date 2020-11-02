package ldap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func randStr() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type UserCache interface {
	Add(gimlet.User) error
	Put(gimlet.User) (string, error)
	Clear(gimlet.User, bool) error
	GetOrCreate(gimlet.User) (gimlet.User, error)
	Get(string) (gimlet.User, bool, error)
	Find(string) (gimlet.User, bool, error)
}

func NewInMemoryUserCache(ctx context.Context, ttl time.Duration) UserCache {
	c := &userCache{
		ttl:         ttl,
		cache:       make(map[string]cacheValue),
		userToToken: make(map[string]string),
	}

	go func() {
		timer := time.NewTimer(ttl / 2)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				c.clean()
				timer.Reset(ttl / 2)
			}
		}
	}()
	return c
}

type cacheValue struct {
	user gimlet.User
	time time.Time
}

type userCache struct {
	mu  sync.RWMutex
	ttl time.Duration

	cache       map[string]cacheValue
	userToToken map[string]string
}

func (c *userCache) clean() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, v := range c.cache {
		if time.Since(v.time) < c.ttl {
			continue
		}
		delete(c.userToToken, v.user.Username())
		delete(c.cache, k)
	}
}

func (c *userCache) Add(u gimlet.User) error {
	_, err := c.Put(u)

	return errors.Wrap(err, "problem adding user")
}

func (c *userCache) Put(u gimlet.User) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := u.Username()
	token := randStr()

	c.userToToken[id] = token
	c.cache[token] = cacheValue{
		user: u,
		time: time.Now(),
	}

	return token, nil
}

func (c *userCache) Get(token string) (gimlet.User, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	u, ok := c.cache[token]
	if !ok {
		return nil, false, nil
	}

	if time.Since(u.time) >= c.ttl {
		return u.user, false, nil
	}

	return u.user, true, nil
}

func (c *userCache) Clear(u gimlet.User, all bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if all {
		c.cache = make(map[string]cacheValue)
		c.userToToken = make(map[string]string)
		return nil
	}

	token, ok := c.userToToken[u.Username()]
	if !ok {
		return errors.New("invalid user")
	}

	delete(c.userToToken, u.Username())
	delete(c.cache, token)

	return nil
}

func (c *userCache) Find(id string) (gimlet.User, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	token, ok := c.userToToken[id]
	if !ok {
		return nil, false, errors.Errorf("could not find user of id %s", id)
	}

	user, exists := c.cache[token]
	if !exists {
		return nil, false, errors.Errorf("could not find user of id %s", id)
	}

	if time.Since(user.time) >= c.ttl {
		return user.user, false, nil
	}

	return user.user, true, nil
}

func (c *userCache) GetOrCreate(u gimlet.User) (gimlet.User, error) {
	usr, _, err := c.Find(u.Username())
	if err == nil {
		return usr, nil
	}

	if err := c.Add(u); err != nil {
		return nil, err
	}

	return u, nil
}

////////////////////////////////////////////////////////////////////////
//
// User cache wrapping external functions

type externalUserCache struct {
	put         PutUserGetToken // Put user to cache
	get         GetUserByToken  // Get user from cache
	clear       ClearUserToken  // Clear user from cache
	find        GetUserByID     // Get user from storage
	getOrCreate GetOrCreateUser // Get or create user from storage
}

func (opts CreationOpts) MakeUserCache() UserCache {
	if opts.UserCache != nil {
		return opts.UserCache
	}

	return &externalUserCache{
		put:         opts.PutCache,
		get:         opts.GetCache,
		clear:       opts.ClearCache,
		find:        opts.GetUser,
		getOrCreate: opts.GetCreateUser,
	}
}

func (c *externalUserCache) Add(u gimlet.User) error                        { _, err := c.getOrCreate(u); return err }
func (c *externalUserCache) Put(u gimlet.User) (string, error)              { return c.put(u) }
func (c *externalUserCache) Get(token string) (gimlet.User, bool, error)    { return c.get(token) }
func (c *externalUserCache) Clear(u gimlet.User, all bool) error            { return c.clear(u, all) }
func (c *externalUserCache) Find(id string) (gimlet.User, bool, error)      { return c.find(id) }
func (c *externalUserCache) GetOrCreate(u gimlet.User) (gimlet.User, error) { return c.getOrCreate(u) }
