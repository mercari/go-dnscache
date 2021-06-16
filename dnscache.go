package dnscache

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

const (
	// defaultCacheSize is initial size of addr and IP list cache map.
	defaultCacheSize = 64
)

// defaultFreq is default frequency a resolver refreshes DNS cache.
var (
	defaultFreq          = 3 * time.Second
	defaultLookupTimeout = 10 * time.Second
)

// lookupIP is a wrapper of net.DefaultResolver.LookupIPAddr.
// This is used to replace lookup function when test.
var lookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}

	ips := make([]net.IP, len(addrs))
	for i, ia := range addrs {
		ips[i] = ia.IP
	}

	return ips, nil
}

// onRefreshed is called when DNS are refreshed.
var onRefreshed = func() {}

// Resolver is DNS cache resolver which cache DNS resolve results in memory.
type Resolver struct {
	lookupIPFn        func(ctx context.Context, host string) ([]net.IP, error)
	dialLookupTimeout time.Duration // dialLookupTimeout is used when DialFunc

	lock      sync.RWMutex
	cache     map[string][]net.IP
	cacheSize int

	refreshLookupTimeout time.Duration // refreshLookupTimeout is used when refreshing DNS cache
	logger               logr.Logger

	closer func()
}

// Option configures a Resolver.
type Option func(r *Resolver)

// WithCacheSize sets cache size to Resolver.
func WithCacheSize(cacheSize int) Option {
	return Option(func(r *Resolver) {
		r.cacheSize = cacheSize
	})
}

// WithLogger sets logger to Resolver.
func WithLogger(logger logr.Logger) Option {
	return Option(func(r *Resolver) {
		r.logger = logger
	})
}

// New initializes DNS cache resolver and starts auto refreshing in a new goroutine.
// To stop refreshing, call `Stop()` function.
func New(freq, lookupTimeout time.Duration, logger *zap.Logger) (*Resolver, error) {
	return NewWithOption(freq, lookupTimeout, WithLogger(zapr.NewLogger(logger))), nil
}

// New initializes DNS cache resolver and starts auto refreshing in a new goroutine.
// To stop refreshing, call `Stop()` function.
func NewWithOption(freq, lookupTimeout time.Duration, opts ...Option) *Resolver {
	if freq <= 0 {
		freq = defaultFreq
	}
	if lookupTimeout <= 0 {
		lookupTimeout = defaultLookupTimeout
	}

	ticker := time.NewTicker(freq)
	ch := make(chan struct{})
	closer := func() {
		ticker.Stop()
		close(ch)
	}

	// copy handler function to avoid race
	onRefreshedFn := onRefreshed

	r := &Resolver{
		lookupIPFn:           lookupIP,
		dialLookupTimeout:    lookupTimeout,
		cacheSize:            defaultCacheSize,
		refreshLookupTimeout: lookupTimeout,
		logger:               logr.Discard(),
		closer:               closer,
	}
	for _, o := range opts {
		o(r)
	}
	r.cache = make(map[string][]net.IP, r.cacheSize)

	go func() {
		for {
			select {
			case <-ticker.C:
				r.Refresh()
				onRefreshedFn()
			case <-ch:
				return
			}
		}
	}()

	return r
}

// LookupIP lookups IP list from DNS server then it saves result in the cache.
// If you want to get result from the cache use `Fetch` function.
func (r *Resolver) LookupIP(ctx context.Context, addr string) ([]net.IP, error) {
	ips, err := r.lookupIPFn(ctx, addr)
	if err != nil {
		return nil, err
	}

	r.lock.Lock()
	r.cache[addr] = ips
	r.lock.Unlock()
	return ips, nil
}

// Fetch fetches IP list from the cache. If IP list of the given addr is not in the cache,
// then it lookups from DNS server by `Lookup` function.
func (r *Resolver) Fetch(ctx context.Context, addr string) ([]net.IP, error) {
	r.lock.RLock()
	ips, ok := r.cache[addr]
	r.lock.RUnlock()
	if ok {
		return ips, nil
	}
	return r.LookupIP(ctx, addr)
}

// Refresh refreshes IP list cache.
func (r *Resolver) Refresh() {
	r.lock.RLock()
	addrs := make([]string, 0, len(r.cache))
	for addr := range r.cache {
		addrs = append(addrs, addr)
	}
	r.lock.RUnlock()

	for _, addr := range addrs {
		ctx, cancelF := context.WithTimeout(context.Background(), r.refreshLookupTimeout)
		if _, err := r.LookupIP(ctx, addr); err != nil {
			r.logger.Error(err, "failed to refresh DNS cache", "addr", addr)
		}
		cancelF()
	}
}

// Stop stops auto refreshing.
func (r *Resolver) Stop() {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.closer != nil {
		r.closer()
		r.closer = nil
	}
}
