package eds

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	logger "github.com/sirupsen/logrus"

	"github.com/beck917/easymesh/utils/eds/registry"
	"github.com/beck917/easymesh/utils/errors"
	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"

	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// adapter implements registry.Discovery interface by wrapping ADS api.
type adapter struct {
	ads           discovery.AggregatedDiscoveryServiceClient
	node          *core.Node
	addr          string
	cc            *grpc.ClientConn
	ccErr         error
	single        *singleflight.Group
	streams       sync.Map
	watcher       registry.Watcher
	cache         *registry.ServiceCache
	fetchInterval time.Duration

	closed uint32
}

func New(addr string) (*adapter, error) {
	return NewWithInterval(addr, time.Minute)
}

func NewWithInterval(addr string, interval time.Duration) (*adapter, error) {
	if interval <= 0 {
		interval = time.Minute
	}

	urlobj, err := url.Parse(addr)
	if err == nil && len(urlobj.Host) > 0 {
		addr = urlobj.Host
	}

	eds := &adapter{
		addr: addr,
		node: &core.Node{
			Id: generateNodeID(),
		},
		single:        new(singleflight.Group),
		cache:         registry.NewServiceCache(nil, 0),
		streams:       sync.Map{},
		fetchInterval: interval,
	}

	err = eds.connect()
	if err != nil {
		return nil, errors.Wrap(err)
	}

	// heart-beat with keep-alive streams
	go eds.loop()

	return eds, nil
}

func (eds *adapter) GetServices(name string, opts ...registry.DiscoveryOption) (services []*registry.Service, err error) {
	if eds.ccErr != nil {
		err = errors.Wrap(eds.ccErr)
		return
	}

	options := registry.NewCommonDiscoveryOption(opts...)
	key := registry.NewServiceKey(name, options.Tags, options.DC)

	// first, try cache
	services, err = eds.cache.GetServices(key)
	if err == nil && len(services) > 0 {
		return
	}

	// second, fetch from eds
	return eds.fetchServices(key, options.Tags)
}

func (eds *adapter) Watch(w registry.Watcher) {
	eds.watcher = w
}

func (eds *adapter) Notify(event registry.Event) {}

func (eds *adapter) Close() {
	if eds.cc == nil {
		return
	}

	eds.ccErr = eds.cc.Close()
	eds.cc = nil
}

func (eds *adapter) loop() {
	timer := time.NewTimer(eds.fetchInterval)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			eds.streams.Range(func(key, value interface{}) bool {
				serviceKey, ok := key.(registry.ServiceKey)
				if !ok {
					//eds.streams.Delete(key)
					return true
				}

				client, ok := value.(discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient)
				if !ok {
					//eds.streams.Delete(key)
					return true
				}

				err := eds.requestStream(client, serviceKey.Name)
				if err != nil {
					logger.Errorf("eds.requestStream(%+v): %+v", serviceKey, err)
					//eds.streams.Delete(key)
				}

				return true
			})

			timer.Reset(eds.fetchInterval)
		}
	}
}

func (eds *adapter) getStreamClient(key registry.ServiceKey) discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient {
	iface, ok := eds.streams.Load(key)
	if ok {
		client, ok := iface.(discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient)
		if ok {
			return client
		}
	}
	return nil
}

func (eds *adapter) createStreamClient(key registry.ServiceKey) (discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient, error) {
	// second, build the new stream and start watch
	meta := metadata.New(map[string]string{
		"DATACENTER": key.DC,
		"TAGS":       key.Tags,
	})

	client, err := eds.ads.StreamAggregatedResources(context.Background(), grpc.Header(&meta))
	if err != nil {
		return nil, err
	}

	eds.streams.Store(key, client)
	return client, nil
}

func (eds *adapter) fetchServices(key registry.ServiceKey, filterTags []string) (services []*registry.Service, err error) {
	iface, err, _ := eds.single.Do(key.ToString(), func() (interface{}, error) {
		// first, trigger request if exists
		iface, ok := eds.streams.Load(key)
		if ok {
			client, ok := iface.(discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient)
			if ok {
				return nil, eds.requestStream(client, key.Name)
			}
		}

		client, err := eds.createStreamClient(key)
		if err != nil {
			return nil, err
		}

		err = eds.requestStream(client, key.Name)
		if err != nil {
			return nil, err
		}

		services, err := eds.parseStream(client, filterTags)
		if err != nil {
			return nil, err
		}

		if len(services) == 0 {
			return nil, errors.ErrNotFound
		}

		eds.cache.Set(key, services)

		if eds.watcher != nil {
			eds.watcher.Handle(key, services)
		}

		eds.startWatch(client, key, filterTags)

		return services, nil
	})

	if err != nil {
		return
	}

	if services, ok := iface.([]*registry.Service); ok {
		return services, nil
	}

	return nil, errors.ErrNotFound
}

func (eds *adapter) startWatch(client discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient, key registry.ServiceKey, tags []string) {
	logger.Infof("eds.Watch(%+v) ...", key)

	go func(streamClient discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient, serviceKey registry.ServiceKey) {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				logger.Errorf("eds.startWatch() panic: %+v", panicErr)
			} else {
				//eds.streams.Delete(key)
			}
		}()

		for {
			services, err := eds.parseStream(eds.getStreamClient(key), tags)
			if err != nil {
				logger.Errorf("eds.Watch(%+v): %+v", key, err)
				time.Sleep(1 * time.Second)
				continue
			}

			if len(services) <= 0 {
				logger.Warnf("eds.Watch(%+v): empty services, ignored!", key)
				continue
			}

			ipv4s := make([]string, len(services))
			for i, svc := range services {
				ipv4s[i] = svc.ServiceIP()
			}
			logger.Infof("eds.Watch(%+v): total=%d, services=%+v", key, len(services), ipv4s)

			eds.cache.Set(key, services)

			if eds.watcher != nil {
				eds.watcher.Handle(key, services)
			}
		}
	}(client, key)
}

func (eds *adapter) requestStream(client discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient, name string) error {
	request := &api.DiscoveryRequest{
		Node:          eds.node,
		VersionInfo:   "v1.0.0",
		ResourceNames: []string{name},
		TypeUrl:       EnvoyClusterLoadAssignment,
	}

	err := client.Send(request)
	if err != nil {
		eds.reconnect(err)
	}

	return err
}

func (eds *adapter) parseStream(client discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient, filterTags []string) (services []*registry.Service, err error) {
	resp, err := client.Recv()
	if err != nil {
		eds.reconnect(err)

		err = errors.Wrap(err)
		return
	}

	err = resp.Validate()
	if err != nil {
		err = errors.Wrap(err)
		return
	}

	services = parseADSResources(resp.Resources)
	if len(filterTags) > 0 {
		var tmpServices []*registry.Service
		for _, service := range services {
			if !stringsIn(service.Tags, filterTags) {
				continue
			}

			tmpServices = append(tmpServices, service)
		}

		services = tmpServices
	}
	return
}

func (eds *adapter) connect() error {
	if eds == nil {
		return errors.New("nil object")
	}

	if eds.cc != nil {
		err := eds.cc.Close()
		if err != nil {
			logger.Errorf("grpc.ClientConn.Close(): %+v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eds.cc, eds.ccErr = grpc.DialContext(ctx, "passthrough:///eds",
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return net.Dial("tcp", eds.addr)
		}))

	if eds.ccErr == nil {
		eds.ads = discovery.NewAggregatedDiscoveryServiceClient(eds.cc)
	} else {
		logger.Errorf("grpc.DialContext(passthrough:///eds, %s): %+v", eds.addr, eds.ccErr)
	}

	return eds.ccErr
}

func (eds *adapter) close() error {
	if !atomic.CompareAndSwapUint32(&eds.closed, 0, 1) {
		return fmt.Errorf("connect already closed")
	}
	return eds.cc.Close()
}

func (eds *adapter) reconnect(err error) {
	if err == nil {
		return
	}

	logger.Warnf("reconnect %s caused by %+v ...", eds.addr, err)

	var shouldReconnect = false

	rpcStatus, rpcOk := status.FromError(err)
	if rpcOk {
		switch rpcStatus.Code() {
		case codes.Unavailable, codes.Canceled, codes.Aborted:
			shouldReconnect = true
		}
	} else {
		switch err.(type) {
		case *net.OpError:
			shouldReconnect = true
		}
	}

	if !shouldReconnect {
		return
	}

	_, err, _ = eds.single.Do(eds.addr, func() (v interface{}, err error) {
		err = eds.connect()
		if err != nil {
			return
		}

		// 重新建立stream
		eds.streams.Range(func(key, value interface{}) bool {
			streamClient, err := eds.createStreamClient(key.(registry.ServiceKey))
			if err != nil {
				logger.Errorf("eds getStreamClient err(%+v): %+v", key, err)
				return false
			}

			// 发送请求，激活watch
			err = eds.requestStream(streamClient, key.(registry.ServiceKey).Name)
			if err != nil {
				return false
			}

			return true
		})
		return
	})

	if err != nil {
		logger.Errorf("connect %s: %+v", eds.addr, err)
	} else {
		logger.Infof("connect %s: OK!", eds.addr)
	}
}
