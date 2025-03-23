package main

import (
	"context"
	"fmt"
	"log"

	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/sourcegraph/conc/pool"
)

func main() {

	stream := make(chan mqtt.Message)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := NewDatabase(true, true)
	if err != nil {
		log.Fatal(err)
	}
	db.Start(ctx)
	defer db.Stop()

	rainfallRepo := repository.NewRainfallRepository(db.Sql)
	fmt.Println(rainfallRepo)
	rainfallRepo.Migrate(ctx)

	go processEvents(stream)
	runMQTT(stream)

}

func processEvents(stream chan mqtt.Message) {
	p := pool.New().WithMaxGoroutines(10)
	for elem := range stream {
		p.Go(func() {
			msg := model.ParseRainfallMessage(elem)

			fmt.Println(msg)

		})
	}
	p.Wait()

}
