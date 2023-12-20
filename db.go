package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/joho/godotenv"
)

var DAY0 = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

func getEnvVar(key string) string {
	err := godotenv.Load(".env-db")

	if err != nil {
		log.Fatal(err)
	}
	return os.Getenv(key)
}

func writePriceToDB(db *sql.DB) {
	for id, p := range preDbPrices {

		// Start a transaction.
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
		// log.Println("Write to DB id:", p.Id)
		// Insert data into the "prices" table within the transaction.
		insertSQL := "INSERT INTO prices (id, price, timestamp, signatures, inserted) VALUES ($1, $2, $3, $4, now())"

		_, err = tx.Exec(insertSQL, p.Id, p.Price, p.Timestamp, pq.Array(p.Signatures))
		if err != nil {
			tx.Rollback()

			if strings.Contains(err.Error(), "pq: duplicate key value violates unique constraint") {
				// the Price Already in DB
				delete(preDbPrices, id)
				continue
			} else {
				log.Println("Error inserting into database:", err)
				log.Fatal(err)
			}

		} else {
			// if all good - Commit the transaction.
			err = tx.Commit()
			if err != nil {
				log.Println("Error committing transaction:", err)
			}
			// Del Price from Cache
			delete(preDbPrices, id)

			return
		}

	}

}

func canWriteToDB_Node(db *sql.DB) bool {
	var nodeName string

	// Only the node with the last hartbeat can write
	query := fmt.Sprintf("SELECT nodename FROM heartbeat ORDER BY timestamp DESC LIMIT 1")
	err := db.QueryRow(query).Scan(&nodeName)
	if err != nil {
		log.Fatal(err)
	}

	if nodeName == config.nodeName {
		return true
	}

	return false
}

func canWriteToDB_Inserted(db *sql.DB) bool {
	var allowed bool

	query := fmt.Sprintf("SELECT now()-inserted > '%d seconds' FROM prices ORDER BY inserted DESC LIMIT 1", DB_WRITE_INTERVAL_SEC)
	err := db.QueryRow(query).Scan(&allowed)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			allowed = true
		} else {
			log.Fatal(err)
		}
	}

	if allowed {
		return true
	}

	return false
}
