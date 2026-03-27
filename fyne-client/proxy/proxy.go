package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type Proxy struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu  sync.Mutex
	vpn *VPNSession
}

func NewProxy() *Proxy {
	return &Proxy{}
}

func (p *Proxy) Start(params *TurnParams, listenAddr, peerAddr string, n int, direct bool) error {
	p.ctx, p.cancel = context.WithCancel(context.Background())

	peer, err := net.ResolveUDPAddr("udp", peerAddr)
	if err != nil {
		return err
	}

	listenConn, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		return err
	}

	context.AfterFunc(p.ctx, func() {
		listenConn.Close()
	})

	listenConnChan := make(chan net.PacketConn)
	go func() {
		for {
			select {
			case <-p.ctx.Done():
				return
			case listenConnChan <- listenConn:
			}
		}
	}()

	t := time.Tick(100 * time.Millisecond)

	if direct {
		for i := 0; i < n; i++ {
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				p.oneTurnConnectionLoop(params, peer, listenConnChan, t)
			}()
		}
	} else {
		okchan := make(chan struct{})
		connchan := make(chan net.PacketConn)

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.oneDtlsConnectionLoop(peer, listenConnChan, connchan, okchan)
		}()

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.oneTurnConnectionLoop(params, peer, connchan, t)
		}()

		select {
		case <-okchan:
		case <-p.ctx.Done():
			return nil
		}

		for i := 0; i < n-1; i++ {
			cchan := make(chan net.PacketConn)
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				p.oneDtlsConnectionLoop(peer, listenConnChan, cchan, nil)
			}()
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				p.oneTurnConnectionLoop(params, peer, cchan, t)
			}()
		}
	}

	return nil
}

// StartWithVPN starts TURN/DTLS proxy and then brings up WireGuard using the provided config,
// overriding the WireGuard endpoint to point at listenAddr.
func (p *Proxy) StartWithVPN(params *TurnParams, listenAddr, peerAddr string, n int, direct bool, wgConfText string) error {
	if wgConfText == "" {
		return fmt.Errorf("wireguard config is empty")
	}
	if err := p.Start(params, listenAddr, peerAddr, n, direct); err != nil {
		return err
	}

	sess, err := StartVPNWithEndpoint(wgConfText, listenAddr)
	if err != nil {
		p.Stop()
		return err
	}

	p.mu.Lock()
	p.vpn = sess
	p.mu.Unlock()
	return nil
}

func (p *Proxy) Stop() {
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
	}

	p.mu.Lock()
	sess := p.vpn
	p.vpn = nil
	p.mu.Unlock()
	if sess != nil {
		sess.Stop()
	}
}

func (p *Proxy) oneDtlsConnectionLoop(peer *net.UDPAddr, listenConnChan <-chan net.PacketConn, connchan chan<- net.PacketConn, okchan chan<- struct{}) {
	for {
		select {
		case <-p.ctx.Done():
			return
		case listenConn := <-listenConnChan:
			c := make(chan error)
			go OneDtlsConnection(p.ctx, peer, listenConn, connchan, okchan, c)
			select {
			case <-p.ctx.Done():
				return
			case err := <-c:
				if err != nil {
					log.Printf("DTLS error: %v", err)
				}
			}
		}
	}
}

func (p *Proxy) oneTurnConnectionLoop(params *TurnParams, peer *net.UDPAddr, connchan <-chan net.PacketConn, t <-chan time.Time) {
	for {
		select {
		case <-p.ctx.Done():
			return
		case conn2 := <-connchan:
			select {
			case <-p.ctx.Done():
				return
			case <-t:
				c := make(chan error)
				go OneTurnConnection(p.ctx, params, peer, conn2, c)
				select {
				case <-p.ctx.Done():
					return
				case err := <-c:
					if err != nil {
						log.Printf("TURN error: %v", err)
					}
				}
			}
		}
	}
}
