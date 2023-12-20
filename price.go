package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

type Price struct {
	Id         uint64   `json:"id"`
	Price      float64  `json:"price"`
	Timestamp  string   `json:"timestamp"`
	Signatures [][]byte `json:"signatures"`
}

func publishPrice(ctx context.Context, topic *pubsub.Topic, p *Price) {
	// Convert to JSON and emit to pubsub
	pByte, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Println("!!!Struct as JSON:", string(newPriceByte))

	if err := topic.Publish(ctx, pByte); err != nil {
		fmt.Println("### Publish error:", err)
	}
}

func signPrice(p *Price, pKey *ecdsa.PrivateKey) {
	messageHash := gethcrypto.Keccak256Hash([]byte(fmt.Sprint(p.Price)))
	signature, err := gethcrypto.Sign(messageHash.Bytes(), pKey)
	if err != nil {
		panic(err)
	}
	p.Signatures = append(p.Signatures, signature)
}

// TODO Should be implemented with the library's functions and PubKey but didn't work
func isPriceSigned(p *Price, pKey *ecdsa.PrivateKey) bool {
	messageHash := gethcrypto.Keccak256Hash([]byte(fmt.Sprint(p.Price)))
	signature, err := gethcrypto.Sign(messageHash.Bytes(), pKey)
	if err != nil {
		panic(err)
	}
	for _, sig := range p.Signatures {
		if bytes.Equal(sig, signature) {
			return true
		}
	}
	return false
}

func getPrice() (float64, string) {
	timestamp := fmt.Sprint(time.Now().Unix())
	price := rand.Float64()

	return price, timestamp
}
