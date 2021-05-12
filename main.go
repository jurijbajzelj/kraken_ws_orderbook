package main

import (
	"kraken_ws_orderbook/data"
	"kraken_ws_orderbook/ws"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Println("Starting...")

	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	mutex := &sync.Mutex{} // mutex should be utilized if graceful shutdown is to be implemented

	books := [data.ExchangesSupported]data.Book{} // readonly copies of the books

	channel_kraken := make(chan data.Book)
	go ws.Kraken(channel_kraken, data.ReadOnlyBookDepth)

	for {
		select {

		case <-sigs:
			log.Println("Received shutdown signal...")
			log.Println("Waiting for the trade to finish...")
			mutex.Lock()
			log.Println("Trade finished, shutting down...")
			syscall.Exit(0)

		case books[data.Kraken] = <-channel_kraken:
			// Based on the orderbook update do appropriate actions such as executing a trade
			log.Println(books[data.Kraken])
		}
	}
}
