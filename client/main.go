// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type getCredsFunc func(string) (string, string, string, error)

func oneDtlsConnectionLoop(ctx context.Context, peer *net.UDPAddr, listenConnChan <-chan net.PacketConn, connchan chan<- net.PacketConn, okchan chan<- struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case listenConn := <-listenConnChan:
			c := make(chan error)
			go oneDtlsConnection(ctx, peer, listenConn, connchan, okchan, c)
			if err := <-c; err != nil {
				log.Printf("%s", err)
			}
		}
	}
}

func oneTurnConnectionLoop(ctx context.Context, turnParams *turnParams, peer *net.UDPAddr, connchan <-chan net.PacketConn, t <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case conn2 := <-connchan:
			select {
			case <-t:
				c := make(chan error)
				go oneTurnConnection(ctx, turnParams, peer, conn2, c)
				if err := <-c; err != nil {
					log.Printf("%s", err)
				}
			default:
			}
		}
	}
}

func main() { //nolint:cyclop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signalChan
		log.Printf("Terminating...\n")
		cancel()
		select {
		case <-signalChan:
		case <-time.After(5 * time.Second):
		}
		log.Fatalf("Exit...\n")
	}()

	host := flag.String("turn", "", "override TURN server ip")
	port := flag.String("port", "", "override TURN port")
	listen := flag.String("listen", "127.0.0.1:9000", "listen on ip:port")
	vklink := flag.String("vk-link", "", "VK calls invite link \"https://vk.com/call/join/...\"")
	yalink := flag.String("yandex-link", "", "Yandex telemost invite link \"https://telemost.yandex.ru/j/...\"")
	peerAddr := flag.String("peer", "", "peer server address (host:port)")
	n := flag.Int("n", 0, "connections to TURN (default 16 for VK, 1 for Yandex)")
	udp := flag.Bool("udp", false, "connect to TURN with UDP")
	direct := flag.Bool("no-dtls", false, "connect without obfuscation. DO NOT USE")
	flag.Parse()
	if *peerAddr == "" {
		log.Panicf("Need peer address!")
	}
	peer, err := net.ResolveUDPAddr("udp", *peerAddr)
	if err != nil {
		panic(err)
	}
	if (*vklink == "") == (*yalink == "") {
		log.Panicf("Need either vk-link or yandex-link!")
	}
	var link string
	var getCreds getCredsFunc
	if *vklink != "" {
		parts := strings.Split(*vklink, "join/")
		link = parts[len(parts)-1]
		getCreds = getVkCreds
		if *n <= 0 {
			*n = 16
		}
	} else {
		parts := strings.Split(*yalink, "j/")
		link = parts[len(parts)-1]
		getCreds = getYandexCreds
		if *n <= 0 {
			*n = 1
		}
	}
	if idx := strings.IndexAny(link, "/?#"); idx != -1 {
		link = link[:idx]
	}
	params := &turnParams{
		*host,
		*port,
		link,
		*udp,
		getCreds,
	}

	listenConnChan := make(chan net.PacketConn)
	listenConn, err := net.ListenPacket("udp", *listen) // nolint: noctx
	if err != nil {
		log.Panicf("Failed to listen: %s", err)
	}
	context.AfterFunc(ctx, func() {
		if closeErr := listenConn.Close(); closeErr != nil {
			log.Panicf("Failed to close local connection: %s", closeErr)
		}
	})
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case listenConnChan <- listenConn:
			}
		}
	}()

	wg1 := sync.WaitGroup{}
	t := time.Tick(100 * time.Millisecond)
	if *direct {
		for i := 0; i < *n; i++ {
			wg1.Go(func() {
				oneTurnConnectionLoop(ctx, params, peer, listenConnChan, t)
			})
		}
	} else {
		okchan := make(chan struct{})
		connchan := make(chan net.PacketConn)

		wg1.Go(func() {
			oneDtlsConnectionLoop(ctx, peer, listenConnChan, connchan, okchan)
		})

		wg1.Go(func() {
			oneTurnConnectionLoop(ctx, params, peer, connchan, t)
		})

		select {
		case <-okchan:
		case <-ctx.Done():
		}
		for i := 0; i < *n-1; i++ {
			connchan := make(chan net.PacketConn)
			wg1.Go(func() {
				oneDtlsConnectionLoop(ctx, peer, listenConnChan, connchan, nil)
			})
			wg1.Go(func() {
				oneTurnConnectionLoop(ctx, params, peer, connchan, t)
			})
		}
	}

	wg1.Wait()
}
