package dnscache

import (
	"context"
	"math/rand"
	"net"
	"time"

	"go.uber.org/zap"
)

var randPerm = func(n int) []int {
	return rand.Perm(n)
}

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// DialFunc is a helper function which returns `net.DialContext` function.
// It randomly fetches an IP from the DNS cache and dials it by the given dial
// function. It dials one by one and returns first connected `net.Conn`.
// If it fails to dial all IPs from cache it returns first error. If no baseDialFunc
// is given, it sets default dial function.
//
// You can use returned dial function for `http.Transport.DialContext`.
//
// In this function, it uses functions from `rand` package. To make it really random,
// you MUST call `rand.Seed` and change the value from the default in your application
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

		beforeFetch := time.Now()
		ips, err := resolver.Fetch(ctxLookup, h)
		if err != nil {
			return nil, err
		}
		afterFetch := time.Now()

		var firstErr error
		for _, randomIndex := range randPerm(len(ips)) {
			ip := ips[randomIndex].String()
			conn, err := baseDialFunc(ctx, "tcp", net.JoinHostPort(ip, p))
			if err == nil {
				if resolver.logger != nil {
					dialTakes := time.Since(afterFetch)
					resolver.logger.Debug("dial with dns cache success", zap.String("addr", addr),
						zap.String("ip", ip), zap.Duration("resolve_takes", afterFetch.Sub(beforeFetch)),
						zap.Duration("dial_takes", dialTakes))
				}
				return conn, nil
			}
			if firstErr == nil {
				firstErr = err
			}
		}

		return nil, firstErr
	}
}
