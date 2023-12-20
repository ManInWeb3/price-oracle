package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	// discovery "github.com/libp2p/go-libp2p/p2p/discovery/routing"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

func initDHT(ctx context.Context, h host.Host) *dht.IpfsDHT {
	kademliaDHT, err := dht.New(ctx, h)
	if err != nil {
		panic(err)
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		panic(err)
	}
	var wg sync.WaitGroup
	bPeers := dht.DefaultBootstrapPeers

	// Add given bootstrap address
	if config.bootstrapAddr != "" {
		targetAddr, err := multiaddr.NewMultiaddr(config.bootstrapAddr)
		if err != nil {
			panic(err)
		}
		bPeers = append(bPeers, targetAddr)
	}

	for _, peerAddr := range bPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Connect(ctx, *peerinfo); err != nil {
				fmt.Println("Bootstrap warning:", err)
			}
		}()
	}
	wg.Wait()

	return kademliaDHT
}

func discoverPeers(ctx context.Context, h host.Host, ns string) {
	kademliaDHT := initDHT(ctx, h)
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, ns)

	connectPeers(ctx, h, ns, routingDiscovery)

	ticker := time.NewTicker(time.Second * SEARCH_PEERS_INTERVAL_SEC)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			connectPeers(ctx, h, ns, routingDiscovery)
		}
	}
	// fmt.Println("Peer discovery complete")
}

func connectPeers(ctx context.Context, h host.Host, ns string, routingDiscovery *drouting.RoutingDiscovery) {
	// fmt.Println("Searching for peers...")
	peerChan, err := routingDiscovery.FindPeers(ctx, ns)
	if err != nil {
		panic(err)
	}
	for peer := range peerChan {
		if peer.ID == h.ID() {
			continue // No self connection
		}
		err := h.Connect(ctx, peer)
		if err != nil {
			// fmt.Printf("Failed connecting to %s\n", peer.ID)
		} else {
			connectedNodes = len(h.Network().Peers())
			// for _, addr := range peer.Addrs {
			// 	fmt.Println(addr)
			// }
		}
	}
}

func listPeers(ctx context.Context, pub *pubsub.PubSub, topic string) {
	ticker := time.NewTicker(time.Second * 60)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			peers := pub.ListPeers(topic)
			fmt.Println("Num of peers: ", len(peers))
			for _, p := range peers {
				fmt.Println("Peerlist: ", p)
			}
		}
	}
}
