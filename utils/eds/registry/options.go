package registry

//=====================discovery=====================
type CommonDiscoveryOption struct {
	DC   string
	Tags []string
}

type DiscoveryOpt func(*CommonDiscoveryOption)

func (DiscoveryOpt) IsDiscovery() {
}

func WithDC(dc string) DiscoveryOpt {
	return func(o *CommonDiscoveryOption) {
		o.DC = dc
	}
}

func WithTags(tags []string) DiscoveryOpt {
	return func(o *CommonDiscoveryOption) {
		o.Tags = tags
	}
}

func NewCommonDiscoveryOption(opts ...DiscoveryOption) *CommonDiscoveryOption {
	o := new(CommonDiscoveryOption)
	for _, opt := range opts {
		switch opt := opt.(type) {
		case DiscoveryOpt:
			opt(o)
		}
	}
	return o
}
