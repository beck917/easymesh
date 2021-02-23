package eds

type Option func(o *option)

type option struct {
	dc   string
	tags []string
}

func WithDatacenter(dc string) Option {
	return func(o *option) {
		o.dc = dc
	}
}

func WithTags(tags ...string) Option {
	return func(o *option) {
		o.tags = tags
	}
}
