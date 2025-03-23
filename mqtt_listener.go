package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func runMQTT(stream chan mqtt.Message) {

	opts := mqtt.NewClientOptions()
	opts.AddBroker("mqtt://mqtt.eclipseprojects.io:1883")
	opts.SetClientID("go_mqtt_client")
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
		c.SubscribeMultiple(map[string]byte{"rainfall": 0}, func(c mqtt.Client, m mqtt.Message) {
			fmt.Printf("Event received: %s from topic:%s\n", m.Payload(), m.Topic())
			stream <- m
		})

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

}
