package registry

import (
	"sync"
	"time"

	"github.com/beck917/easymesh/utils/errors"
	logger "github.com/sirupsen/logrus"
)

// Cacher represents cache interface of service.
type Cacher interface {
	LastModify(ServiceKey) (time.Time, error)
	Store(ServiceKey, interface{}) error
}

type ServiceCache struct {
	store        sync.Map
	cacher       Cacher
	syncInterval time.Duration
}

func NewServiceCache(cacher Cacher, interval time.Duration) *ServiceCache {
	if cacher != nil && interval.Hours() < 1 {
		interval = time.Hour
	}

	return &ServiceCache{
		cacher:       cacher,
		syncInterval: interval,
	}
}

func (c *ServiceCache) Set(key ServiceKey, services []*Service) {
	_, ok := c.load(key)
	if !ok {
		c.store.Store(key, services)

		if c.cacher != nil {
			if err := c.cacher.Store(key, services); err != nil {
				logger.Errorf("%T.Store(%s, %d): %v", c.cacher, key, len(services), err)
			}
		}
		return
	}

	c.store.Store(key, services)

	// verify cache
	if c.cacher == nil {
		return
	}

	lastModify, err := c.cacher.LastModify(key)
	if err != nil {
		if errors.Is(err, errors.ErrNotFound) {
			if err := c.cacher.Store(key, services); err != nil {
				logger.Errorf("%T.Store(%s, %d): %v", c.cacher, key, len(services), err)
			}
		} else {
			logger.Errorf("%T.LastModify(%s): %v", c.cacher, key, err)
		}

		return
	}

	if lastModify.Add(c.syncInterval).After(time.Now()) {
		return
	}

	if err := c.cacher.Store(key, services); err != nil {
		logger.Errorf("%T.Store(%s, %d): %v", c.cacher, key, len(services), err)
	}
}

func (c *ServiceCache) GetServices(key ServiceKey) ([]*Service, error) {
	services, ok := c.load(key)
	if !ok {
		return nil, errors.Wrap(errors.ErrNotFound)
	}

	return services, nil
}

func (c *ServiceCache) load(key ServiceKey) (services []*Service, ok bool) {
	val, ok := c.store.Load(key)
	if !ok {
		return
	}

	services, ok = val.([]*Service)
	return
}

// Flush 清空缓存
func (c *ServiceCache) Flush() error {
	c.store.Range(func(key, value interface{}) bool {
		c.store.Delete(key)

		return true
	})
	return nil
}
