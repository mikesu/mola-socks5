package main

import (
	"context"
	"github.com/mikesu/mola-socks5"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	signalChan := make(chan os.Signal)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGQUIT)
	ctx, cancel := context.WithCancel(context.Background())
	go socks5.Run(ctx, "127.0.0.1:3080")
	select {
	case s := <-signalChan:
		log.Println("rcv signal: ", s)
		cancel()
	}
}
