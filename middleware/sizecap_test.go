package middleware

import (
	"context"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// fakeMqttMessage is a raw MQTT message for the raw-message stage.
type fakeMqttMessage struct {
	topic   string
	payload []byte
}

func (f fakeMqttMessage) Duplicate() bool   { return false }
func (f fakeMqttMessage) Qos() byte         { return 0 }
func (f fakeMqttMessage) Retained() bool    { return false }
func (f fakeMqttMessage) Topic() string     { return f.topic }
func (f fakeMqttMessage) Payload() []byte   { return f.payload }
func (f fakeMqttMessage) Ack()              {}
func (f fakeMqttMessage) MessageID() uint16 { return 0 }

func TestSizeCapDropsOversized(t *testing.T) {
	sizeCap := SizeCap(SizeCapDefault)

	var passed int
	handler := sizeCap(func(ctx context.Context, msg mqtt.Message) { passed++ })

	handler(context.Background(), fakeMqttMessage{topic: "rainfall", payload: make([]byte, SizeCapDefault)})
	if passed != 1 {
		t.Fatalf("at-cap message passed = %d, want 1", passed)
	}
	handler(context.Background(), fakeMqttMessage{topic: "rainfall", payload: make([]byte, SizeCapDefault+1)})
	if passed != 1 {
		t.Fatalf("oversized message reached the handler, passed = %d", passed)
	}
}
