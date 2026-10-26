package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/doug-benn/mqtt-logger/batch"
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

	opts := mqtt.NewClientOptions()
	opts.AddBroker(utilities.GetEnvOrDefault("MQTT_BROKER", "mqtt://broker.connectmq.com:1883"))
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
		//"temperature": 1,
		if token := c.SubscribeMultiple(map[string]byte{"temperature": 0, "rainfall": 0}, func(c mqtt.Client, m mqtt.Message) {
			fmt.Printf("Event received: %s from topic:%s\n", m.Payload(), m.Topic())
			pool.Submit(func() {
				processMQTT(m, tempBatch, rainfallRepo)
			})
		}); token.Wait() && token.Error() != nil {
			fmt.Println("Subscription failed:", token.Error())
		}
	}

	opts.DefaultPublishHandler = func(c mqtt.Client, m mqtt.Message) {
		fmt.Printf("Received message: %s from topic: %s\n", m.Payload(), m.Topic())
	}

	client := mqtt.NewClient(opts)

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
