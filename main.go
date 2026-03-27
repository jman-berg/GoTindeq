package main

import (
	"log"

	"github.com/jman-berg/gotindeq/internal/tindeq"
)

func main() {
	client := tindeq.NewTindeqClient()
	err := client.Connect()
	if err != nil {
		log.Fatalln("Failed to connect to Tindeq: ", err)
	}
	if err := client.DiscoverServices(); err != nil {
		log.Fatalln("Failed to discover Progressor Services:", err)
	}
}
