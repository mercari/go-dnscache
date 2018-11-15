package dnscache

import (
	"context"
	"math/rand"
	"net"
	"sync"
	"time"
)

var randPerm = defaultPerm()

// defaultPerm returns perm function that is safe for
// concurrent use.
// Since a source from rand.NewSource is not concurrent safe,
// protect perm function from concurrent call by locking.
func defaultPerm() func(int) []int {
	var (
		mu sync.Mutex
		r  = rand.New(rand.NewSource(time.Now().UnixNano()))
	)
	return func(n int) (is []int) {
		mu.Lock()
		is = r.Perm(n)
		mu.Unlock()
		return
	}
}

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// DialFunc is a helper function which returns `net.DialContext` function.
// It randomly fetches an IP from the DNS cache and dial it by the given dial
// function. It dials one by one and return first connected `net.Conn`.
// If it fails to dial all IPs from cache it returns first error. If no baseDialFunc
// is given, it sets default dial function.
//
// You can use returned dial function for `http.Transport.DialContext`.
func DialFunc(resolver *Resolver, baseDialFunc dialFunc) dialFunc {
	if baseDialFunc == nil {
		// This is same as which `http.DefaultTransport` uses.
		baseDialFunc = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		h, p, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		// Fetch DNS result from cache.
		//
		// ctxLookup is only used for cancelling DNS Lookup.
		ctxLookup, cancelF := context.WithTimeout(ctx, resolver.lookupTimeout)
		defer cancelF()
		ips, err := resolver.Fetch(ctxLookup, h)
		if err != nil {
			return nil, err
		}

		var firstErr error
		for _, randomIndex := range randPerm(len(ips)) {
			conn, err := baseDialFunc(ctx, "tcp", net.JoinHostPort(ips[randomIndex].String(), p))
			if err == nil {
				return conn, nil
			}
			if firstErr == nil {
				firstErr = err
			}
		}

		return nil, firstErr
	}
}
