package eds

import (
	"fmt"
	"os"
	"strings"

	"github.com/beck917/easymesh/utils/eds/registry"
	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/uuid"
	"github.com/hashicorp/go-sockaddr"
	logger "github.com/sirupsen/logrus"
)

func generateNodeID() string {
	ip, err := sockaddr.GetPrivateIP()
	if err != nil {
		ip, err = os.Hostname()
		if err != nil {
			ip = uuid.New().String()
		}
	}

	return ip
}

func isSidecarOffline(meta map[string]string) bool {
	if meta == nil {
		return false
	}

	status := meta["status"]

	return status == "offline"
}

func parseADSResources(resources []*any.Any) (services []*registry.Service) {
	for _, resource := range resources {
		var assignment api.ClusterLoadAssignment

		err := proto.Unmarshal(resource.GetValue(), &assignment)
		if err != nil {
			logger.Errorf("proto.Unmarshal(%s, %s): %v", resource.GetValue(), assignment.String(), err)
			continue
		}

		// for endpoints
		for _, endpoint := range assignment.GetEndpoints() {
			for _, lbendpoint := range endpoint.GetLbEndpoints() {
				socket := lbendpoint.GetEndpoint().GetAddress().GetSocketAddress()
				weight := lbendpoint.GetLoadBalancingWeight()

				meta := map[string]string{}

				var tags []string

				for key, value := range lbendpoint.GetMetadata().FilterMetadata {
					// for tags
					switch key {
					case "istio_ext":
						for k, v := range value.GetFields() {
							if k != "tags" {
								continue
							}

							tags = strings.Split(v.GetStringValue(), ",")
						}
					default:
						for k, v := range value.GetFields() {
							meta[k] = v.GetStringValue()
						}
					}
				}

				// skip of sidecar offline
				if isSidecarOffline(meta) {
					continue
				}

				serviceID := meta["id"]
				if len(serviceID) == 0 {
					serviceID = fmt.Sprintf("%s~%s", assignment.GetClusterName(), socket.GetAddress())
				}

				services = append(services, &registry.Service{
					ID:     serviceID,
					Name:   assignment.GetClusterName(),
					IP:     socket.GetAddress(),
					Port:   int(socket.GetPortValue()),
					Weight: int32(weight.GetValue()),
					Tags:   tags,
					Meta:   meta,
				})
			}
		}

		// for named endpoints
		for name, endpoint := range assignment.GetNamedEndpoints() {
			socket := endpoint.GetAddress().GetSocketAddress()

			services = append(services, &registry.Service{
				ID:     fmt.Sprintf("%s~%s", name, socket.GetAddress()),
				Name:   name,
				IP:     socket.GetAddress(),
				Port:   int(socket.GetPortValue()),
				Weight: 100,
			})
		}
	}

	return
}

func stringsIn(actualKeys, expectedKeys []string) bool {
	n := len(expectedKeys)
	if n == 0 {
		return true
	}

	if len(actualKeys) == 0 {
		return false
	}

	matched := make(map[string]bool)
	for _, key := range expectedKeys {
		matched[key] = false
	}
	for _, key := range actualKeys {
		_, ok := matched[key]
		if ok {
			matched[key] = true
		}
	}

	for _, ok := range matched {
		if !ok {
			return false
		}
	}

	return true
}
