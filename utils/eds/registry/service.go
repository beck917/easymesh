package registry

import (
	"sort"
	"strings"
	"sync"

	"github.com/beck917/easymesh/utils/errors"
)

// Service
type Service struct {
	ID         string            `discovery:"可选,服务id"`
	Name       string            `discovery:"必填,服务名"`
	IP         string            `discovery:"可选,默认拿en0的地址"`
	IPTemplate string            `discovery:"可选,可以用于指定特殊网卡" json:"IPTemplate,omitempty"`
	Tags       []string          `discovery:"可选,标签"`
	Port       int               `discovery:"必填,端口"`
	Weight     int32             `discovery:"可选,权重"`
	Meta       map[string]string `discovery:"可选,自定义元数据"`

	once sync.Once
}

func (s *Service) ServiceIP() string {
	return s.IP
}

// ServiceKey defines service with query meta
type ServiceKey struct {
	Name string
	Tags string
	DC   string
}

func NewServiceKey(name string, tags []string, dc string) ServiceKey {
	sort.Strings(tags)

	tag := strings.Join(tags, ":")

	return ServiceKey{
		Name: name,
		Tags: tag,
		DC:   dc,
	}
}

func (key *ServiceKey) ToString() string {
	fields := make([]string, 0)
	if len(key.Tags) > 0 {
		fields = append(fields, key.Tags)
	}

	fields = append(fields, key.Name)
	fields = append(fields, "service")

	if len(key.DC) > 0 {
		fields = append(fields, key.DC)
	}

	return strings.Join(fields, ".")

}

// key formatted in [<tags>.]<service name>.service.[.<consul datacenter>]
func ParseServiceKey(key string) (*ServiceKey, error) {
	fields := strings.Split(key, ".")

	findService := func(fields []string) int {
		for i, field := range fields {
			if field == "service" {
				return i
			}
		}
		return -1
	}

	idx := findService(fields)
	if idx <= 1 {
		return nil, errors.Wrap(errors.ErrArgument)
	}

	serviceKey := &ServiceKey{}

	serviceKey.Name = fields[idx-1]
	if idx-1 > 0 {
		serviceKey.Tags = strings.Join(fields[:idx-1], ".")
	}
	if idx+1 > len(fields) {
		serviceKey.DC = strings.Join(fields[idx+1:], ".")
	}

	return serviceKey, nil
}
