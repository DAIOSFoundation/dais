package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"time"

	"../core"
	"../types"
	datastore "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-floodsub"
	"github.com/libp2p/go-libp2p"
	libp2pdht "github.com/libp2p/go-libp2p-kad-dht"
)

var wg sync.WaitGroup

func main() {
	wg.Add(1)
	MineTopic := "daiosMine"
	PeerTopic := "daiosPeer"
	list := make(map[string]types.Address)
	/*
		list["ED968E840D"] = types.NewAddress("ED968E840D")
		list["61C5F9D848"] = types.NewAddress("61C5F9D848")
		list["EBDD83C1B1"] = types.NewAddress("EBDD83C1B1")
	*/
	ctx := context.Background()
	core.ValidatorPool = []types.Address{}

	host, err := libp2p.New(ctx)
	if err != nil {
		panic(err)
	}
	for _, addr := range host.Addrs() {
		fmt.Printf("%s/ipfs/%s\n", addr.String(), host.ID().Pretty())
	}

	fsub, err := floodsub.NewFloodSub(ctx, host)
	if err != nil {
		panic(err)
	}

	dht := libp2pdht.NewDHT(ctx, host, datastore.NewMapDatastore())
	if err != nil {
		panic(err)
	}

	bsConfig := libp2pdht.DefaultBootstrapConfig
	bsConfig.Period = 10 * time.Second
	bsConfig.Queries = 1000
	if _, err := dht.BootstrapWithConfig(bsConfig); err != nil {
		panic(err)
	}

	go func() {
		for {
			core.Pick()

		}
	}()

	go func() {
		for {
			broadCastAddr := core.BroadCastAddr()
			b := <-*broadCastAddr
			jsonBytes, err := json.Marshal(b)
			if err != nil {
				panic(err)
			}
			if err := fsub.Publish(MineTopic, jsonBytes); err != nil {
				panic(err)
			}
		}
	}()

	subPeer, err := fsub.Subscribe(PeerTopic)
	if err != nil {
		panic(err)
	}

	go func() {
		for {

			msg, err := subPeer.Next(ctx)
			if err != nil {
				panic(err)
			}

			var addr types.Address
			json.Unmarshal(msg.GetData(), &addr)

			list[string(addr[:])] = addr

			core.ValidatorPool = append(core.ValidatorPool, addr)

			time.Sleep(1 * time.Second)

		}

	}()
	wg.Wait()
}
