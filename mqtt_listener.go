package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/doug-benn/mqtt-logger/batch"
	"github.com/doug-benn/mqtt-logger/handlers"
	"github.com/doug-benn/mqtt-logger/middleware"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
	"github.com/doug-benn/mqtt-logger/utilities"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func runMQTT(rainfallRepo repository.RainfallRepo) {
	pool := pond.NewPool(100)

	tempBatch := batch.NewBatchProcessor(10, 60*time.Second, func(batch []model.TemperatureMessage) error {
		fmt.Println("Processing Batch")
		for _, item := range batch {
			fmt.Printf("Batch item: %+v\n", item)
		}
		time.Sleep(10 * time.Second)
		return nil
	})

	// The route set is the wiring point: each topic declares its composed
	// handler, and the broker subscription derives from it — adding a topic
	// is one entry here.
	routes := buildRoutes(rainfallRepo, tempBatch)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(utilities.GetEnvOrDefault("MQTT_BROKER", "mqtt://localhost:1883"))
	opts.SetClientID(utilities.GetEnvOrDefault("MQTT_CLIENT_ID", "go_mqtt_client"))
	opts.AutoReconnect = true
	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		fmt.Println("Connection Lost:", err)
	}

	opts.OnReconnecting = func(c mqtt.Client, co *mqtt.ClientOptions) {
		fmt.Println("Reconnecting...")
	}
	opts.OnConnect = func(c mqtt.Client) {
		fmt.Println("MQTT Client Connected")
		// The routes carry the handlers; this subscription only tells the
		// broker what to send.
		if token := c.SubscribeMultiple(subscribeTopics(routes), nil); token.Wait() && token.Error() != nil {
			fmt.Println("Subscription failed:", token.Error())
		}
	}

	opts.DefaultPublishHandler = func(c mqtt.Client, m mqtt.Message) {
		fmt.Printf("Dropping message from unhandled topic %s: %s\n", m.Topic(), m.Payload())
	}

	client := mqtt.NewClient(opts)

	// paho dispatches topic routes through its own router; every message is
	// processed on the pool so callbacks stay non-blocking and a slow DB
	// write never stalls intake. This loop is the only place the MQTT
	// library feeds a handler chain: everything a chain sees is the raw
	// message built here.
	for topic, handler := range routes {
		client.AddRoute(topic, func(c mqtt.Client, m mqtt.Message) {
			msg := handlers.RawMessage{Topic: m.Topic(), Payload: m.Payload()}
			pool.Submit(func() {
				handler(context.Background(), msg)
			})
		})
	}

	fmt.Println("Connecting to MQTT Broker...")
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// Wait for interrupt signal to gracefully shutdown the subscriber
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	// Unsubscribe and disconnect
	fmt.Println("Unsubscribing and disconnecting...")
	client.Disconnect(250)

	// Stop the pool and wait for all submitted tasks to complete
	pool.StopAndWait()

	// Flush the last partial batch; without this, queued readings are dropped
	if err := tempBatch.Close(); err != nil {
		fmt.Println("Error closing batch processor:", err)
	}
}

// buildRoutes composes each topic's chain: its handler wrapped in that
// topic's middleware stages, first stage outermost. Policy is declared
// inline per topic, so one entry shows a topic's whole pipeline.
func buildRoutes(rainfallRepo repository.RainfallRepo, tempBatch *batch.BatchProcessor[model.TemperatureMessage]) map[string]handlers.Handler[handlers.RawMessage] {
	return map[string]handlers.Handler[handlers.RawMessage]{
		"rainfall": middleware.Chain(
			handlers.RainfallHandler(rainfallRepo),
			middleware.SizeCap(middleware.SizeCapDefault),
		),
		"temperature": middleware.Chain(
			handlers.TemperatureHandler(tempBatch),
			middleware.SizeCap(middleware.SizeCapDefault),
		),
	}
}

// subscribeTopics is the broker subscription for a route set: exactly the
// topics the logger handles, all at QoS 0.
func subscribeTopics(routes map[string]handlers.Handler[handlers.RawMessage]) map[string]byte {
	topics := make(map[string]byte, len(routes))
	for topic := range routes {
		topics[topic] = 0
	}
	return topics
}
