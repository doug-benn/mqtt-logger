package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/alitto/pond/v2"
	"github.com/doug-benn/mqtt-logger/handlers"
	"github.com/doug-benn/mqtt-logger/middleware"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
	"github.com/doug-benn/mqtt-logger/utilities"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"golang.org/x/time/rate"
)

func runMQTT(repo repository.Store, logger *slog.Logger) {
	pool := pond.NewPool(100)

	// The route set is the wiring point: each topic declares its composed
	// handler, and the broker subscription derives from it — adding a topic
	// is one entry here. The set also hands back closeBuffers — the one
	// piece of chain lifecycle wiring must trigger, after the pool drains.
	routes, closeBuffers := buildRoutes(repo, logger)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(utilities.GetEnvOrDefault("MQTT_BROKER", "mqtt://localhost:1883"))
	opts.SetClientID(utilities.GetEnvOrDefault("MQTT_CLIENT_ID", "go_mqtt_client"))
	opts.AutoReconnect = true
	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		logger.Error("connection lost", "err", err)
	}

	opts.OnReconnecting = func(c mqtt.Client, co *mqtt.ClientOptions) {
		logger.Info("reconnecting")
	}
	opts.OnConnect = func(c mqtt.Client) {
		logger.Info("mqtt client connected")
		// The routes carry the handlers; this subscription only tells the
		// broker what to send.
		if token := c.SubscribeMultiple(subscribeTopics(routes), nil); token.Wait() && token.Error() != nil {
			logger.Error("subscription failed", "err", token.Error())
		}
	}

	opts.DefaultPublishHandler = func(c mqtt.Client, m mqtt.Message) {
		logger.Warn(fmt.Sprintf("dropping message from unhandled topic: %s", m.Payload()),
			"topic", m.Topic())
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

	logger.Info("connecting to mqtt broker")
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// Wait for interrupt signal to gracefully shutdown the subscriber
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	// Unsubscribe and disconnect
	logger.Info("unsubscribing and disconnecting")
	client.Disconnect(250)

	// Stop the pool and wait for all submitted tasks to complete
	pool.StopAndWait()

	// Close every buffering chain; without this, queued readings are dropped
	closeBuffers()
}

// route pairs a topic's composed handler with its lifecycle close (nil for a
// chain that buffers nothing): one literal registers a route completely, so
// the dispatch map and closeBuffers both derive from this list and no
// second entry can be forgotten.
type route struct {
	topic   string
	handler handlers.Handler[handlers.RawMessage]
	close   func()
}

// buildRoutes assembles the route set: one literal per topic — its composed
// chain and its close together — from which the dispatch map and the
// shutdown hook are derived. closeBuffers fans the closes out in parallel,
// so shutdown lasts about as long as the slowest flush rather than the sum
// of every chain's.
func buildRoutes(repo repository.Store, logger *slog.Logger) (map[string]handlers.Handler[handlers.RawMessage], func()) {
	temperature, closeTemperature := handlers.NewTemperatureHandler(repo, logger)
	humidity, closeHumidity := handlers.NewHumidityHandler(repo, logger)

	entries := []route{
		{"rainfall", middleware.Chain(
			handlers.RainfallHandler(repo, logger),
			middleware.SizeCap(logger, middleware.SizeCapDefault),
			middleware.Changed(rainfallKey),
			middleware.RateLimit[handlers.RawMessage](rate.NewLimiter(middleware.DefaultRate, middleware.DefaultBurst)),
		), nil},
		{"temperature", temperature, closeTemperature},
		{"humidity", humidity, closeHumidity},
	}

	routes := make(map[string]handlers.Handler[handlers.RawMessage], len(entries))
	closes := make([]func(), 0, len(entries))
	for _, e := range entries {
		routes[e.topic] = e.handler
		if e.close != nil {
			closes = append(closes, e.close)
		}
	}

	closeBuffers := func() {
		var wg sync.WaitGroup
		for _, closeChain := range closes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				closeChain()
			}()
		}
		wg.Wait()
	}
	return routes, closeBuffers
}

// rainfallKey is the change gate's key: the decoded value, so payload
// formatting differences never defeat the gate. A malformed payload keys to
// NaN, which never equals itself — repeats pass through to the handler, where
// every malformed payload gets its own drop log instead of being swallowed
// by the gate.
func rainfallKey(msg handlers.RawMessage) float64 {
	reading, err := model.ParseRainfallMessage(msg.Payload)
	if err != nil {
		return math.NaN()
	}
	return reading.Rainfall
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
