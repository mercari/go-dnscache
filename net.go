package dnscache

import (
	"context"
	"math/rand"
	"net"
	"time"
)

var randPerm = func(n int) []int {
	return rand.Perm(n)
}

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// DialFunc is a helper function which returns `net.DialContext` function.
// It randomly fetches an IP from the dns cache and dial it by the given dial
// function. It dials one by one and return first connected `net.Conn`.
// If it fails to dial all IPs from cache it returns first error. If no baseDialFunc
// is given, it sets default dial function.
//
// You can use returned dial function for `http.Transport.DialContext`.
func DialFunc(resolver *Resolver, baseDialFunc dialFunc) dialFunc {
	rand.Seed(time.Now().UTC().UnixNano())

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
