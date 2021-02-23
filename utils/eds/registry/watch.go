package registry

// Watcher represents a callback for service key.
type Watcher interface {
	Handle(ServiceKey, []*Service)
}

// WatchFunc wraps given func as Watcher interface.
type WatchFunc func(ServiceKey, []*Service)

func (f WatchFunc) Handle(serviceKey ServiceKey, services []*Service) {
	f(serviceKey, services)
}
