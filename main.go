package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	//"os"
	"sync"
	"time"

	"./com"
	"./core"
	"./types"
	"github.com/davecgh/go-spew/spew"
	cid "github.com/ipfs/go-cid"
	datastore "github.com/ipfs/go-datastore"
	ipfsaddr "github.com/ipfs/go-ipfs-addr"
	"github.com/libp2p/go-floodsub"
	"github.com/libp2p/go-libp2p"
	libp2pdht "github.com/libp2p/go-libp2p-kad-dht"
	net "github.com/libp2p/go-libp2p-net"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	"github.com/multiformats/go-multihash"
)

var daios *core.Daios
var mutex sync.Mutex
var i int
var wg sync.WaitGroup

func main() {
	wg.Add(1)
	dhtKey := "daios"
	blockTopic := "daiosBlock"
	txTopic := "daiosTx"
	PeerTopic := "daiosPeer"
	MineTopic := "daiosMine"

	ctx := context.Background()

	host, err := libp2p.New(ctx)
	if err != nil {
		panic(err)
	}

	fsub, err := floodsub.NewGossipSub(ctx, host)
	if err != nil {
		panic(err)
	}

	address := types.NewAddress(com.Hash(string(host.ID())))

	daios = core.New(address)
	mutex = *daios.Mutex()

	dht := libp2pdht.NewDHTClient(ctx, host, datastore.NewMapDatastore())
	if err != nil {
		panic(err)
	}

	bsConfig := libp2pdht.DefaultBootstrapConfig
	bsConfig.Period = 10 * time.Second
	bsConfig.Queries = 1000
	if _, err := dht.BootstrapWithConfig(bsConfig); err != nil {
		panic(err)
	}

	bootstrapNode := []string{
		"/ip4/192.168.12.5/tcp/54112/ipfs/QmTQXbzviYxr1QV3LQGjZfoD3E3H2wb3FbByKM8xYX7YUx",
	}

	for _, addr := range bootstrapNode {
		iaddr, err := ipfsaddr.ParseString(addr)
		if err != nil {
			panic(err)
		}

		pinfo, err := peerstore.InfoFromP2pAddr(iaddr.Multiaddr())
		if err != nil {
			panic(err)
		}

		if err := host.Connect(ctx, *pinfo); err != nil {
			fmt.Println(err)

		}
	}

	c, _ := cid.NewPrefixV1(cid.Raw, multihash.SHA2_256).Sum([]byte(dhtKey))

	tctx, _ := context.WithTimeout(ctx, time.Second*10)
	if err := dht.Provide(tctx, c, true); err != nil {
		panic(err)
	}

	tctx, _ = context.WithTimeout(ctx, time.Second*10)
	peers, err := dht.FindProviders(tctx, c)
	if err != nil {
		panic(err)
	}

	if err != nil {
		panic(err)
	}

	host.SetStreamHandler("/p2p/1.0.0", handleStream)

	for _, p := range peers {
		if p.ID == host.ID() {
			continue
		}

		tctx, _ := context.WithTimeout(ctx, time.Second*5)

		if err := host.Connect(tctx, p); err != nil {
			fmt.Println("failed to connect to peer: ", p.ID)
		} else {
			fmt.Println("connect to peer : ", p.ID)

			s, err := host.NewStream(context.Background(), p.ID, "/p2p/1.0.0")
			if err != nil {
				log.Fatalln(err)
			}

			rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
			mutex.Lock()

			response, err := rw.ReadString('\n')
			if err != nil {
				log.Println(err)
			}

			var b core.BlockChain

			json.Unmarshal([]byte(response), &b)
			daios.BlockChain().SyncChain(&b)

			mutex.Unlock()

		}
	}

	jsonBytes, err := json.Marshal(address)
	if err != nil {
		panic(err)
	}
	if err := fsub.Publish(PeerTopic, jsonBytes); err != nil {
		panic(err)
	}

	subBlock, err := fsub.Subscribe(blockTopic)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			msg, err := subBlock.Next(ctx)
			if err != nil {
				panic(err)
			}

			var b types.Block

			daios.TxPool().Remove()
			json.Unmarshal(msg.GetData(), &b)
			daios.BlockChain().AddBlock(&b)
			spew.Dump(b.States)
			sdb := *types.CS.DB()
			sdb.Sync(b.States)

			jsonChain, err := json.Marshal(daios.BlockChain())
			if err != nil {
				panic(err)
			}

			err = ioutil.WriteFile("./chain.json", jsonChain, 0600)
			if err != nil {
				fmt.Println(err)
			}

		}
	}()

	go func() {
		for {

			broadCastBlocks := core.BroadCastBlocks()
			b := <-*broadCastBlocks
			i = 0

			if err := fsub.Publish(blockTopic, b.MarshalJSON()); err != nil {
				panic(err)
			}

		}
	}()

	subTx, err := fsub.Subscribe(txTopic)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			msg, err := subTx.Next(ctx)
			if err != nil {
				panic(err)
			}
			var tx types.Transaction
			json.Unmarshal(msg.GetData(), &tx)

			txp := *daios.TxPool()
			txp.Enqueue(&tx)

		}
	}()

	subMine, err := fsub.Subscribe(MineTopic)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			msg, err := subMine.Next(ctx)
			if err != nil {
				panic(err)
			}

			var addr types.Address

			json.Unmarshal(msg.GetData(), &addr)

			if bytes.Equal(address[:], addr[:]) {
				core.Mine(daios, address)
			}

		}
	}()

	go func() {

		for {
			time.Sleep(1 * time.Second)
			b, err := ioutil.ReadFile("./makers_logs.json")
			if err != nil {
				fmt.Println(err)
			}

			if b != nil {
				err = ioutil.WriteFile("./makers_logs.json", nil, 0)
				if err != nil {
					fmt.Println(err)
				}
			}

			var data []types.SNSData

			json.Unmarshal(b, &data)

			for _, txdata := range data {
				mutex.Lock()

				d := types.NewTransaction(types.NewAddress(""), types.NewAddress(""), 0, txdata)
				d.Nonce = i
				d.Data.Hash = d.Hash()
				mutex.Unlock()

				if err := fsub.Publish(txTopic, d.MarshalJSON()); err != nil {
					panic(err)
				}
				i += 1

			}

		}

	}()
	wg.Wait()

}

func handleStream(s net.Stream) {
	defer s.Close()

	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
	mutex.Lock()
	bc := *daios.BlockChain()
	rw.WriteString(string(bc.MarshalJSON()) + "\n")
	rw.Flush()
	mutex.Unlock()

}
