package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v3"
)

func handleConnection(ctx context.Context, wg1 *sync.WaitGroup, conn net.Conn, connectAddr string) {
	defer wg1.Done()
	defer conn.Close()
	var err error = nil
	log.Printf("Connection from %s\n", conn.RemoteAddr())

	ctx1, cancel1 := context.WithTimeout(ctx, 30*time.Second)
	dtlsConn, ok := conn.(*dtls.Conn)
	if !ok {
		log.Println("Type error")
		cancel1()
		return
	}
	log.Println("Start handshake")
	if err = dtlsConn.HandshakeContext(ctx1); err != nil {
		log.Println(err)
		cancel1()
		return
	}
	cancel1()
	log.Println("Handshake done")

	serverConn, err := net.Dial("udp", connectAddr)
	if err != nil {
		log.Println(err)
		return
	}
	defer func() {
		if err = serverConn.Close(); err != nil {
			log.Printf("failed to close outgoing connection: %s", err)
			return
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	ctx2, cancel2 := context.WithCancel(ctx)
	context.AfterFunc(ctx2, func() {
		conn.SetDeadline(time.Now())
		serverConn.SetDeadline(time.Now())
	})
	go func() {
		defer wg.Done()
		defer cancel2()
		buf := make([]byte, 1600)
		for {
			select {
			case <-ctx2.Done():
				return
			default:
			}
			conn.SetReadDeadline(time.Now().Add(time.Minute * 30))
			n, err1 := conn.Read(buf)
			if err1 != nil {
				log.Printf("Failed: %s", err1)
				return
			}

			serverConn.SetWriteDeadline(time.Now().Add(time.Minute * 30))
			_, err1 = serverConn.Write(buf[:n])
			if err1 != nil {
				log.Printf("Failed: %s", err1)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		defer cancel2()
		buf := make([]byte, 1600)
		for {
			select {
			case <-ctx2.Done():
				return
			default:
			}
			serverConn.SetReadDeadline(time.Now().Add(time.Minute * 30))
			n, err1 := serverConn.Read(buf)
			if err1 != nil {
				log.Printf("Failed: %s", err1)
				return
			}

			conn.SetWriteDeadline(time.Now().Add(time.Minute * 30))
			_, err1 = conn.Write(buf[:n])
			if err1 != nil {
				log.Printf("Failed: %s", err1)
				return
			}
		}
	}()
	wg.Wait()
	log.Printf("Connection closed: %s\n", conn.RemoteAddr())
}
