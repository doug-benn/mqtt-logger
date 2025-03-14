package main

import (
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func runMQTT() {

	opts := mqtt.NewClientOptions()
	opts.AddBroker("mqtt://test.mosquitto.org:1883")
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
	}

	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
}
