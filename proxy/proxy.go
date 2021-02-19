// Package proxy is a cli proxy
package proxy

import (
	"os"
	"strings"

	"github.com/beck917/easymesh/wrapper/ratelimiter"
	"github.com/micro/go-micro"
	"github.com/micro/go-micro/client"
	"github.com/micro/go-micro/config/options"
	"github.com/micro/go-micro/proxy"
	"github.com/micro/go-micro/proxy/grpc"
	"github.com/micro/go-micro/proxy/http"
	"github.com/micro/go-micro/proxy/mucp"
	"github.com/micro/go-micro/registry"
	"github.com/micro/go-micro/router"
	rs "github.com/micro/go-micro/router/service"
	"github.com/micro/go-micro/server"
	sgrpc "github.com/micro/go-micro/server/grpc"
	smucp "github.com/micro/go-micro/server/mucp"
	"github.com/micro/go-micro/util/log"
	"github.com/micro/go-micro/util/mux"
)

var (
	// Name of the proxy
	Name = "go.micro.proxy"
	// Address The address of the proxy
	Address = ":8081"
	// Protocol the proxy protocol
	Protocol = "http"
	// Endpoint The endpoint host to route to
	Endpoint = "http://127.0.0.1:6060"
)

// ServerOptions server options
type ServerOptions struct {
	Name     string
	Address  string
	Protocol string
	Endpoint string
}

// Run proxy server
func Run(sOpts *ServerOptions, srvOpts ...micro.Option) {
	log.Name("proxy")

	if sOpts.Name != "" {
		Name = sOpts.Name
	}
	if sOpts.Address != "" {
		Address = sOpts.Address
	}
	if sOpts.Protocol != "" {
		Protocol = sOpts.Protocol
	}
	if sOpts.Endpoint != "" {
		Endpoint = sOpts.Endpoint
	}

	// service opts
	srvOpts = append(srvOpts, micro.Name(Name))

	// set address
	if len(Address) > 0 {
		srvOpts = append(srvOpts, micro.Address(Address))
	}

	// set the context
	var popts []options.Option

	// create new router
	var r router.Router

	routerName := ""
	routerAddr := ""

	ropts := []router.Option{
		router.Id(server.DefaultId),
		router.Client(client.DefaultClient),
		router.Address(routerAddr),
		router.Registry(registry.DefaultRegistry),
	}

	// check if we need to use the router service
	switch {
	case routerName == "go.micro.router":
		r = rs.NewRouter(ropts...)
	case len(routerAddr) > 0:
		r = rs.NewRouter(ropts...)
	default:
		r = router.NewRouter(ropts...)
	}

	// start the router
	if err := r.Start(); err != nil {
		log.Logf("Proxy error starting router: %s", err)
		os.Exit(1)
	}

	popts = append(popts, proxy.WithRouter(r))

	// new proxy
	var p proxy.Proxy
	var s server.Server
	// set endpoint
	if len(Endpoint) > 0 {
		switch {
		case strings.HasPrefix(Endpoint, "grpc://"):
			ep := strings.TrimPrefix(Endpoint, "grpc://")
			popts = append(popts, proxy.WithEndpoint(ep))
			p = grpc.NewProxy(popts...)
		case strings.HasPrefix(Endpoint, "http://"):
			// TODO: strip prefix?
			popts = append(popts, proxy.WithEndpoint(Endpoint))
			p = http.NewProxy(popts...)
			s = smucp.NewServer(
				server.WrapHandler(ratelimiter.NewHandlerWrapper(10)),
			)
		default:
			// TODO: strip prefix?
			popts = append(popts, proxy.WithEndpoint(Endpoint))
			p = mucp.NewProxy(popts...)
		}
	}

	// set based on protocol
	if p == nil && len(Protocol) > 0 {
		switch Protocol {
		case "http":
			p = http.NewProxy(popts...)
			// TODO: http server
		case "grpc":
			p = grpc.NewProxy(popts...)
			s = sgrpc.NewServer()
		default:
			p = mucp.NewProxy(popts...)
			s = smucp.NewServer()
		}
	}

	if len(Endpoint) > 0 {
		log.Logf("Proxy [%s] serving endpoint: %s", p.String(), Endpoint)
	} else {
		log.Logf("Proxy [%s] serving protocol: %s", p.String(), Protocol)
	}

	// prepend the server
	if s != nil {
		srvOpts = append([]micro.Option{micro.Server(s)}, srvOpts...)
	}

	// new service
	service := micro.NewService(srvOpts...)

	// create a new proxy muxer which includes the debug handler
	muxer := mux.New(Name, p)

	// set the router
	service.Server().Init(
		server.WithRouter(muxer),
	)

	// Run internal service
	if err := service.Run(); err != nil {
		log.Fatal(err)
	}
}
