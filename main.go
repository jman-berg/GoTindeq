package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jman-berg/gotindeq/internal/tindeq"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := tindeq.NewTindeqClient()
	if err != nil {
		log.Fatalln("Failed to set up Tindeq client: ", err)
	}
	defer client.Close()

	if err := client.SendCommand(client.Commands.TARE_SCALE); err != nil {
		log.Fatalf("Failed to tare scale: %v\n", err)
	}

	if err := client.EnableNotifcations(); err != nil {
		log.Fatalln("Error enabling notifications...", err)
	}

	if err := client.SendCommand(client.Commands.START_WEIGHT_MEAS); err != nil {
		log.Fatalln("Error sending command: ", err)
	}

	<-ctx.Done()
	log.Println("Shutdown signal received. Cleaning up...")

	_, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	log.Println("Application terminated gracefully.")
}
