package registry

type Event string

const (
	EventDegrade Event = "Degrade"
	EventRecover Event = "Recover"
)

// Discovery represents resolver interface.
type Discovery interface {
	GetServices(string, ...DiscoveryOption) ([]*Service, error)
	Notify(event Event)
	Watch(Watcher)
	Close()
}

type DiscoveryOption interface {
	IsDiscovery()
}
