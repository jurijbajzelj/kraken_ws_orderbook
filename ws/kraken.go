package ws

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"kraken_ws_orderbook/data"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

func GetChecksumInput(bids []data.Level, asks []data.Level) string {
	var str strings.Builder
	for _, level := range asks[:10] {
		price := level.Price.StringFixed(5)
		price = strings.Replace(price, ".", "", 1)
		price = strings.TrimLeft(price, "0")
		str.WriteString(price)

		volume := level.Volume.StringFixed(8)
		volume = strings.Replace(volume, ".", "", 1)
		volume = strings.TrimLeft(volume, "0")
		str.WriteString(volume)
	}
	for _, level := range bids[:10] {
		price := level.Price.StringFixed(5)
		price = strings.Replace(price, ".", "", 1)
		price = strings.TrimLeft(price, "0")
		str.WriteString(price)

		volume := level.Volume.StringFixed(8)
		volume = strings.Replace(volume, ".", "", 1)
		volume = strings.TrimLeft(volume, "0")
		str.WriteString(volume)
	}
	return str.String()
}

func verifyOrderBookChecksum(bids []data.Level, asks []data.Level, checksum string) {
	checksumInput := GetChecksumInput(bids, asks)
	crc := crc32.ChecksumIEEE([]byte(checksumInput))
	if fmt.Sprint(crc) != checksum {
		log.Fatal("not the same ", " ", crc, " ", checksum)
	}
}

func getPriceAndVolume(ino interface{}) (decimal.Decimal, decimal.Decimal, int) {
	el := ino.([]interface{})
	price, err := decimal.NewFromString(el[0].(string))
	if err != nil {
		log.Fatal(err)
	}
	volume, err := decimal.NewFromString(el[1].(string))
	if err != nil {
		log.Fatal(err)
	}
	return price, volume, len(el)
}

func Kraken(update_channel chan data.Book, depth int) {
	defer close(update_channel)

	c, _, err := websocket.DefaultDialer.Dial("wss://ws.kraken.com", http.Header{})
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	var init_resp map[string]interface{}
	err = c.ReadJSON(&init_resp)
	if err != nil {
		log.Fatal(err)
	}
	if !(init_resp["event"] == "systemStatus" && init_resp["status"] == "online") {
		log.Fatal(init_resp)
	}

	payload := fmt.Sprintf("{\"event\": \"subscribe\", \"pair\": [\"XBT/USD\"], \"subscription\": {\"name\": \"book\", \"depth\": %d}}", depth)
	c.WriteMessage(1, []byte(payload))
	err = c.ReadJSON(&init_resp)
	if err != nil {
		log.Fatal(err)
	} else if !(init_resp["event"] == "subscriptionStatus" && init_resp["pair"] == "XBT/USD" && init_resp["status"] == "subscribed") {
		log.Fatal(init_resp)
	}

	resp := []interface{}{}
	msg := []byte{}
	heartbeat := []byte{123, 34, 101, 118, 101, 110, 116, 34, 58, 34, 104, 101, 97, 114, 116, 98, 101, 97, 116, 34, 125}

	for { // get initial state of the book
		_, msg, err = c.ReadMessage()
		if err != nil {
			log.Fatal(err)
		} else if bytes.Compare(heartbeat, msg) != 0 {
			// not a heartbeat message
			err := json.Unmarshal(msg, &resp)
			if err != nil {
				log.Fatal(err)
			}
			break
		}
	}

	bids := data.CreateInitial(resp[1].(map[string]interface{}), "bs")
	asks := data.CreateInitial(resp[1].(map[string]interface{}), "as")
	update_channel <- data.Book{Ws_received: time.Now(), Bids: bids[:depth], Asks: asks[:depth]}

	// listen for the incremental updates
	for {
		_, msg, err = c.ReadMessage()
		if err != nil {
			log.Fatal("Kraken c.ReadMessage() err | ", err)
		} else if bytes.Compare(heartbeat, msg) != 0 {
			// not a heartbeat message
			err := json.Unmarshal(msg, &resp)
			if err != nil {
				log.Fatal(err)
			} else {

				ws_received := time.Now()
				if len(resp) == 4 {

					var order_book_diff map[string]interface{} = resp[1].(map[string]interface{})
					checksum := order_book_diff["c"].(string)

					if val, ok := order_book_diff["b"]; ok {

						for _, el_interface := range val.([]interface{}) {
							price, volume, length := getPriceAndVolume(el_interface)
							if volume.Equals(decimal.Zero) {
								bids = data.RemovePriceFromBids(bids, price)
							} else {
								if length == 4 { // it has the 4th element "r" so we just re-append
									bids = append(bids, data.Level{Price: price, Volume: volume})
								} else {
									bids = data.InsertPriceInBids(bids, price, volume)
									bids = bids[:depth]
								}
							}
						}
					} else {

						for _, el_interface := range order_book_diff["a"].([]interface{}) {
							price, volume, length := getPriceAndVolume(el_interface)
							if volume.Equals(decimal.Zero) {
								asks = data.RemovePriceFromAsks(asks, price)
							} else {
								if length == 4 { // it has the 4th element "r" so we just re-append
									asks = append(asks, data.Level{Price: price, Volume: volume})
								} else {
									asks = data.InsertPriceInAsks(asks, price, volume)
									asks = asks[:depth]
								}
							}
						}
					}
					verifyOrderBookChecksum(bids, asks, checksum)
				} else {

					order_book_diff_asks := resp[1].(map[string]interface{})
					order_book_diff_bids := resp[2].(map[string]interface{})
					checksum := order_book_diff_bids["c"].(string)

					for _, el_interface := range order_book_diff_bids["b"].([]interface{}) {
						price, volume, length := getPriceAndVolume(el_interface)
						if volume.Equals(decimal.Zero) {
							bids = data.RemovePriceFromBids(bids, price)
						} else {
							if length == 4 { // it has the 4th element "r" so we just re-append
								bids = append(bids, data.Level{Price: price, Volume: volume})
							} else {
								bids = data.InsertPriceInBids(bids, price, volume)
								bids = bids[:depth]
							}
						}
					}
					for _, el_interface := range order_book_diff_asks["a"].([]interface{}) {
						price, volume, length := getPriceAndVolume(el_interface)
						if volume.Equals(decimal.Zero) {
							asks = data.RemovePriceFromAsks(asks, price)
						} else {
							if length == 4 { // it has the 4th element "r" so we just re-append
								asks = append(asks, data.Level{Price: price, Volume: volume})
							} else {
								asks = data.InsertPriceInAsks(asks, price, volume)
								asks = asks[:depth]
							}
						}
					}
					verifyOrderBookChecksum(bids, asks, checksum)
				}
				update_channel <- data.Book{Ws_received: ws_received, Bids: bids[:depth], Asks: asks[:depth]}
			}
		}
	}
}
