package main

import (
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"golang.org/x/exp/maps"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sony/sonyflake"

	"github.com/go-resty/resty/v2"
)

const (
	SEARCH_PEERS_INTERVAL_SEC = 5
	HEARTBEAT_INTERVAL_SEC    = 5 // Could be calculated from DB_WRITE_INTERVAL_SEC
	MIN_SIGNATURES_NUM        = 3
	GET_PRICE_INTERVAL_SEC    = 30
	DB_WRITE_INTERVAL_SEC     = 30 // How often can write to DB
)

type Config struct {
	sourcePort       int
	nodeName         string
	bootstrapAddr    string
	help             bool
	topicName        string
	RendezvousString string
	minNodes         int
}

var (
	config         Config
	PrivKey        *ecdsa.PrivateKey
	db             *sql.DB
	preDbPrices    map[uint64]Price
	idGenerator    *sonyflake.Sonyflake
	myPrice        Price
	connectedNodes int
)

func init() {

	flag.StringVar(&config.nodeName, "n", "node1", "Node name")
	flag.IntVar(&config.sourcePort, "s", 0, "Source port number")
	flag.StringVar(&config.bootstrapAddr, "b", "", "Bootstrap multiaddr string")
	flag.BoolVar(&config.help, "help", false, "Display help")
	flag.StringVar(&config.topicName, "topicName", "price", "name of topic to join")
	flag.StringVar(&config.RendezvousString, "rendezvousString", "upshotoracle", "rendezvousString")
	flag.IntVar(&config.minNodes, "nodes", 3, "Source port number")

	//flag.BoolVar(&config.debug, "debug", false, "Debug generates the same node ID on every execution")
	flag.Parse()

	if config.help {
		fmt.Printf("This program demonstrates a simple oracle using libp2p\n\n")
		fmt.Println("Usage: Run './oracle -n <ORACLE_NAME>' where <ORACLE_NAME> can be any string.")
		os.Exit(0)
	}

	rand.Seed(time.Now().UnixNano())
	var st sonyflake.Settings
	st.MachineID = func() (uint16, error) {
		return uint16(rand.Intn(1000)), nil
	}

	idGenerator = sonyflake.NewSonyflake(st)
	if idGenerator == nil {
		panic("Sonyflake not created")
	}

	preDbPrices = make(map[uint64]Price)
}

func main() {

	//###########
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var PrivKey = initKeys(fmt.Sprintf("%s.priv", config.nodeName))

	// Need to convert ecdsa.PrivateKey to libp2p  PrivKey
	// PrivKeyByte := gethcrypto.FromECDSA(PrivKey)
	// PrivKeyP2P, _, err := crypto.ECDSAKeyPairFromKey(PrivKey) // x509: unsupported elliptic curve
	// if err != nil {
	// 	panic(err)
	// }

	host, err := libp2p.New(
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", config.sourcePort)),
		// libp2p.Identity(PrivKeyP2P),
		// Attempt to open ports using uPNP for NATed hosts.
		libp2p.NATPortMap(),
	)
	if err != nil {
		panic(err)
	}

	defer host.Close()

	for _, addr := range host.Addrs() {
		fmt.Printf("Listening on %s/p2p/%s\n", addr, host.ID())
	}

	// setup peer discovery
	go discoverPeers(ctx, host, config.RendezvousString)

	fmt.Printf("Connecting to peers...")
	for {
		if connectedNodes >= config.minNodes {
			break
		} else {
			fmt.Printf("... ")
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Printf("\n")
	// Connect to the DB
	// Read database connection details from .env-db file.
	dbName := getEnvVar("DB_NAME")
	dbUser := getEnvVar("DB_USER")
	dbPassword := getEnvVar("DB_PASSWORD")
	dbHost := getEnvVar("DB_HOST")
	dbPort := getEnvVar("DB_PORT")
	connStr := fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		dbUser, dbPassword, dbName, dbHost, dbPort,
	)

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Run node's heartbeat
	go heartbeat(ctx, db)
	// create a new PubSub service using the GossipSub router
	gossipSub, err := pubsub.NewGossipSub(ctx, host)
	if err != nil {
		panic(err)
	}

	// join the pubsub topic
	topic, err := gossipSub.Join(config.topicName)
	if err != nil {
		panic(err)
	}
	defer topic.Close()

	// create publisher
	go publish(ctx, topic, PrivKey)

	// subscribe to topic
	subscriber, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	go subscribe(ctx, topic, subscriber, PrivKey, host.ID())

	go writeToDB(ctx, db)

	// go listPeers(ctx, gossipSub, config.topicName)

	select {}

}

// start publisher to topic
func publish(ctx context.Context, topic *pubsub.Topic, pk *ecdsa.PrivateKey) {

	ticker := time.NewTicker(time.Second * GET_PRICE_INTERVAL_SEC)
	defer ticker.Stop()

	pubPrice := func() {
		// price, timestamp := GetLatestEthereumPrice()
		price, timestamp := getPrice()
		id, err := idGenerator.NextID()
		if err != nil {
			panic(err)
		}
		fmt.Println("Publish new price: ", id, price)

		newPrice := Price{
			Id:        id,
			Price:     price,
			Timestamp: timestamp,
		}

		// Sign newPrice
		signPrice(&newPrice, pk)

		publishPrice(ctx, topic, &newPrice)
	}

	pubPrice()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pubPrice()
		}

	}
}

// start subsriber to topic
func subscribe(ctx context.Context, topic *pubsub.Topic, subscriber *pubsub.Subscription, pk *ecdsa.PrivateKey, hostID peer.ID) {
	for {
		// On new received message
		msg, err := subscriber.Next(ctx)
		if err != nil {
			panic(err)
		}

		// only consider messages delivered by other peers
		if msg.ReceivedFrom == hostID {
			continue
		}

		// Decode JSON to Price
		var receivedPrice Price
		if err := json.Unmarshal(msg.Message.Data, &receivedPrice); err != nil {
			panic(err)
		}
		// fmt.Println("Received price: ", receivedPrice.Id, receivedPrice.Price)

		if len(receivedPrice.Signatures) < MIN_SIGNATURES_NUM {
			if !isPriceSigned(&receivedPrice, pk) {
				// Sign the price
				signPrice(&receivedPrice, pk)
			}
		}

		// Check how many signatures it has now
		if len(receivedPrice.Signatures) >= MIN_SIGNATURES_NUM {
			// cache to preDbPrice
			addPreDBPrice(ctx, receivedPrice, db)
		} else {
			// Publish only if it has not enough signatures
			publishPrice(ctx, topic, &receivedPrice)
		}
	}
}

func addPreDBPrice(ctx context.Context, p Price, db *sql.DB) {
	// Add the given Price to the preDbPrices if it wasnot there
	if _, ok := preDbPrices[p.Id]; !ok {
		preDbPrices[p.Id] = p
	}

	// Update myPrice with the latest available
	// Latest == largest Id, due to Id generation approach https://github.com/sony/sonyflake#sonyflake
	var largestId, k uint64
	for k, _ = range preDbPrices {
		if largestId < k {
			largestId = k
		}
	}

	// myPrice.Id can only increase
	if myPrice.Id < largestId {
		myPrice.Id = largestId
		myPrice.Price = preDbPrices[largestId].Price
		fmt.Printf("Changed myPrice: %f , id: %d\n", myPrice.Price, myPrice.Id)
	}

	var found uint64
	// Prune already saved to DB Prices from the cache
	for id, _ := range preDbPrices {

		query := fmt.Sprintf("SELECT id FROM prices WHERE id=%d", id)
		err := db.QueryRow(query).Scan(&found)
		if err == nil {
			// Del Price from Cache
			delete(preDbPrices, id)
		}
	}
}

func heartbeat(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(HEARTBEAT_INTERVAL_SEC * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			updatePulsInDb(db)
		}
	}
}

func updatePulsInDb(db *sql.DB) {
	// Insert a locks
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if p := recover(); p != nil {
			// If there's a panic, rollback the transaction.
			tx.Rollback()
			panic(p)
		}
	}()

	// Add RANDOMness to the heartbeat timestamps to rotate write nodes
	_, err = tx.Exec(`INSERT INTO heartbeat (nodename, timestamp)
										VALUES ($1, now())
										ON CONFLICT (nodename)
										DO UPDATE SET timestamp = now() + floor(random()*20)*'1 seconds'::interval;`,
		config.nodeName)

	if err != nil {
		tx.Rollback()
		log.Fatal("Error inserting into database:", err)
	}

	err = tx.Commit()
	if err != nil {
		log.Fatal("Error committing transaction:", err)
	}
}

func writeToDB(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// If I can write to DB
			if len(preDbPrices) > 0 && canWriteToDB_Inserted(db) && canWriteToDB_Node(db) {
				writePriceToDB(db)
			}
		}
	}
}

func listPreDbPrices(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 15)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Println("Cached Prices:", maps.Keys(preDbPrices))
		}
	}
}

// CoinGeckoResponse represents the structure of the CoinGecko API response.
type CoinGeckoResponse struct {
	Ethereum struct {
		USD float64 `json:"usd"`
	} `json:"ethereum"`
}

// GetLatestEthereumPrice retrieves the latest Ethereum price from the CoinGecko API.
func GetLatestEthereumPrice() (float64, string) {
	// CoinGecko API endpoint for Ethereum
	apiURL := "https://api.coingecko.com/api/v3/simple/price?ids=ethereum&vs_currencies=usd"

	// Create a new Resty client
	client := resty.New()

	// Make GET request to the CoinGecko API
	response, err := client.R().
		Get(apiURL)
	if err != nil {
		panic(err)
	}

	// Check if the HTTP request was successful (status code 200)
	if response.StatusCode() != 200 {
		panic(err)
	}

	// Parse the JSON response
	var geckoResponse CoinGeckoResponse
	if err := json.Unmarshal(response.Body(), &geckoResponse); err != nil {
		panic(err)
	}

	// Retrieve the Ethereum price from the response
	ethereumPrice := geckoResponse.Ethereum.USD
	timestamp := fmt.Sprint(time.Now().Unix())
	return ethereumPrice, timestamp
}
