package main

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"os"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
)

func initKeys(PrivKeyFile string) *ecdsa.PrivateKey {
	// Reads or Generates new key pair
	var err error
	var PrivKey *ecdsa.PrivateKey

	_, errPriv := os.Stat(PrivKeyFile)
	// _, errPub := os.Stat(PubKeyFile)
	if errPriv == nil {
		// Keys exists
		PrivKey, err = gethcrypto.LoadECDSA(PrivKeyFile)
		if err != nil {
			log.Fatal(err)
		}

	} else {
		// Generate new keys
		var err error
		PrivKey, err = gethcrypto.GenerateKey()
		if err != nil {
			log.Fatal(err)
		}

		err = gethcrypto.SaveECDSA(PrivKeyFile, PrivKey)
		if err != nil {
			log.Fatal(err)
		}
		log.Println(fmt.Sprintf("Key generated, and saved: %s", PrivKeyFile))

	}

	return PrivKey
}
