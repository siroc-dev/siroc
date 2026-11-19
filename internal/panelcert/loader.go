package panelcert

import (
	"crypto/tls"
	"os"
	"sync"
	"time"
)

// NewLoader reloads the certificate from disk when the file changes,
// so a renewal is used on the next connection.
func NewLoader(certFile, keyFile string) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	var state struct {
		sync.Mutex
		mod  time.Time
		size int64
		cert *tls.Certificate
	}
	return func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		info, err := os.Stat(certFile)
		if err != nil {
			return nil, err
		}
		state.Lock()
		defer state.Unlock()
		if state.cert != nil && info.ModTime().Equal(state.mod) && info.Size() == state.size {
			return state.cert, nil
		}
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		state.mod = info.ModTime()
		state.size = info.Size()
		state.cert = &pair
		return state.cert, nil
	}
}
