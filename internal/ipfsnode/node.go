package ipfsnode

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ipfs/boxo/bitswap"
	bsnet "github.com/ipfs/boxo/bitswap/network"
	"github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/gateway"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
)

// Node represents an embedded IPFS node
type Node struct {
	host       host.Host
	dht        *dht.IpfsDHT
	blockstore *Blockstore
	bswap      *bitswap.Bitswap
	dagService format.DAGService
	gateway    *http.Server

	// Root CIDs to advertise
	rootCIDs []cid.Cid

	// For periodic re-advertising
	ctx    context.Context
	cancel context.CancelFunc
}

// Config for the IPFS node
type Config struct {
	ListenAddrs    []string // libp2p listen addresses
	AnnounceAddrs  []string // Addresses to announce to the network (public IPs)
	GatewayAddr    string   // HTTP gateway address (e.g., ":8080")
	BootstrapPeers []string // Bootstrap peer addresses
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		ListenAddrs: []string{
			"/ip4/0.0.0.0/tcp/4001",
			"/ip4/0.0.0.0/udp/4001/quic-v1",
		},
		GatewayAddr: ":8080",
		BootstrapPeers: []string{
			// IPFS/libp2p official bootstrap nodes
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
			"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
			"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
			"/ip4/104.131.131.82/udp/4001/quic-v1/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",

			// Protocol Labs nodes
			"/dnsaddr/am6.bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
			"/dnsaddr/ny5.bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
			"/dnsaddr/sg1.bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
			"/dnsaddr/sv15.bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",

			// Cloudflare IPFS nodes
			"/ip4/172.65.0.13/tcp/4001/p2p/QmcfgsJsMtx6qJb74akCw1M24X1zFwgGo11h1cuhwQjtJP",
			"/ip6/2606:4700:60::6/tcp/4001/p2p/QmcfgsJsMtx6qJb74akCw1M24X1zFwgGo11h1cuhwQjtJP",

			// Pinata nodes
			"/dnsaddr/bitswap.pinata.cloud/p2p/Qma8ddFEQWEU8ijWvdxXm3nxU7oHsRtCykAaVz8WUYhiKn",

			// NFT.Storage / web3.storage nodes
			"/ip4/145.40.96.233/tcp/4001/p2p/12D3KooWEGeZ19Q79NdzS6CJBoCwFZwujqi5hoK8BtRcLa48fJdu",
			"/ip4/147.75.87.85/tcp/4001/p2p/12D3KooWBnmsaeNRP6SCdNbhzaNHihQQBPDhmDvjVGsR1EbswncV",
		},
	}
}

// NewNode creates a new IPFS node
func NewNode(ctx context.Context, storage StorageBackend, cfg *Config) (*Node, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Create blockstore
	blockstore := NewBlockstore(storage)

	// Parse listen addresses
	var listenAddrs []multiaddr.Multiaddr
	for _, addr := range cfg.ListenAddrs {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid listen address %s: %w", addr, err)
		}
		listenAddrs = append(listenAddrs, ma)
	}

	// Parse announce addresses (public IPs to advertise)
	var announceAddrs []multiaddr.Multiaddr
	for _, addr := range cfg.AnnounceAddrs {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid announce address %s: %w", addr, err)
		}
		announceAddrs = append(announceAddrs, ma)
	}

	// Create connection manager
	connMgr, err := connmgr.NewConnManager(100, 400, connmgr.WithGracePeriod(time.Minute))
	if err != nil {
		return nil, fmt.Errorf("failed to create connection manager: %w", err)
	}

	// Build libp2p options
	opts := []libp2p.Option{
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.ConnectionManager(connMgr),
		libp2p.NATPortMap(),                    // Enable UPnP/NAT-PMP
		libp2p.EnableNATService(),              // Help others with NAT detection
		libp2p.EnableHolePunching(),            // Enable hole punching for NAT traversal
		libp2p.EnableRelayService(),            // Act as relay for others
		libp2p.EnableAutoRelayWithStaticRelays( // Use relays if needed
			dht.GetDefaultBootstrapPeerAddrInfos(),
		),
	}

	// Add announce addresses if configured (for servers with public IPs)
	if len(announceAddrs) > 0 {
		opts = append(opts, libp2p.AddrsFactory(func([]multiaddr.Multiaddr) []multiaddr.Multiaddr {
			return announceAddrs
		}))
	}

	// Add DHT routing
	var dhtInstance *dht.IpfsDHT
	opts = append(opts, libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		var err error
		dhtInstance, err = dht.New(ctx, h, dht.Mode(dht.ModeServer))
		return dhtInstance, err
	}))

	// Create libp2p host with DHT and NAT traversal
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Bootstrap DHT
	if err := dhtInstance.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	// Connect to bootstrap peers
	for _, addrStr := range cfg.BootstrapPeers {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}
		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			continue
		}
		go func(pi peer.AddrInfo) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.Connect(ctx, pi)
		}(*peerInfo)
	}

	// Create bitswap network and exchange
	bsNetwork := bsnet.NewFromIpfsHost(h, dhtInstance)
	bswap := bitswap.New(ctx, bsNetwork, blockstore)

	// Create block service and DAG service
	blockService := blockservice.New(blockstore, bswap)
	dagService := merkledag.NewDAGService(blockService)

	nodeCtx, cancel := context.WithCancel(context.Background())
	node := &Node{
		host:       h,
		dht:        dhtInstance,
		blockstore: blockstore,
		bswap:      bswap,
		dagService: dagService,
		ctx:        nodeCtx,
		cancel:     cancel,
	}

	// Start HTTP gateway if configured
	if cfg.GatewayAddr != "" {
		if err := node.startGateway(cfg.GatewayAddr); err != nil {
			node.Close()
			return nil, fmt.Errorf("failed to start gateway: %w", err)
		}
	}

	// Start periodic re-advertiser (DHT provider records expire)
	go node.periodicAdvertise()

	return node, nil
}

// periodicAdvertise re-advertises root CIDs every 12 hours
func (n *Node) periodicAdvertise() {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			if len(n.rootCIDs) > 0 {
				fmt.Printf("Re-advertising %d root CIDs to DHT...\n", len(n.rootCIDs))
				n.AdvertiseRoots(n.ctx)
			}
		}
	}
}

func (n *Node) startGateway(addr string) error {
	// Create gateway backend
	backend, err := gateway.NewBlocksBackend(
		blockservice.New(n.blockstore, n.bswap),
		gateway.WithValueStore(n.dht),
	)
	if err != nil {
		return err
	}

	// Create gateway handler
	gwHandler := gateway.NewHandler(gateway.Config{
		DeserializedResponses: true,
	}, backend)

	mux := http.NewServeMux()
	mux.Handle("/ipfs/", gwHandler)

	n.gateway = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go n.gateway.ListenAndServe()
	return nil
}

// AddRootCID adds a CID to be advertised to the DHT
func (n *Node) AddRootCID(c cid.Cid) {
	n.rootCIDs = append(n.rootCIDs, c)
}

// AdvertiseRoots advertises all root CIDs to the DHT
func (n *Node) AdvertiseRoots(ctx context.Context) error {
	// Wait for DHT to be ready (need peers to advertise to)
	fmt.Printf("Waiting for DHT peers before advertising...\n")
	for i := 0; i < 30; i++ {
		if n.dht.RoutingTable().Size() > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	fmt.Printf("DHT routing table has %d peers\n", n.dht.RoutingTable().Size())

	for _, c := range n.rootCIDs {
		fmt.Printf("Advertising CID to DHT: %s\n", c)
		if err := n.dht.Provide(ctx, c, true); err != nil {
			fmt.Printf("Warning: failed to provide %s: %v\n", c, err)
			// Continue with other CIDs
		} else {
			fmt.Printf("Successfully advertised: %s\n", c)
		}
	}
	return nil
}

// PeerID returns the node's peer ID
func (n *Node) PeerID() peer.ID {
	return n.host.ID()
}

// Addrs returns the node's listen addresses
func (n *Node) Addrs() []multiaddr.Multiaddr {
	return n.host.Addrs()
}

// Close shuts down the node
func (n *Node) Close() error {
	if n.cancel != nil {
		n.cancel()
	}
	if n.gateway != nil {
		n.gateway.Close()
	}
	if n.bswap != nil {
		n.bswap.Close()
	}
	if n.dht != nil {
		n.dht.Close()
	}
	if n.host != nil {
		n.host.Close()
	}
	return nil
}
