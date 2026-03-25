package resolve

type ResolverRegistry struct {
	resolvers map[string]Resolver
}

var registry *ResolverRegistry

func init() {
	registry = &ResolverRegistry{
		resolvers: make(map[string]Resolver),
	}
	f := FileResolver{}
	e := EnvResolver{}
	registry.register(&f)
	registry.register(&e)
}

func (r *ResolverRegistry) register(resolver Resolver) {
	r.resolvers[resolver.getResolverKey()] = resolver
}

func GetResolver(key string) Resolver {
	return registry.resolvers[key]
}
