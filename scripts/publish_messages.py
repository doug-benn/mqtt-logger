# /// script
# requires-python = ">=3.10"
# dependencies = ["paho-mqtt>=2.0"]
# ///
"""Publish a rainfall and a temperature message once per second."""

import json
import time

import paho.mqtt.client as mqtt

BROKER = "localhost"
PORT = 1883


def main() -> None:
    client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id="dev-publisher")
    client.connect(BROKER, PORT)
    client.loop_start()
    print(f"publishing rainfall + temperature to {BROKER}:{PORT} every second")

    n = 0
    try:
        while True:
            n += 1
            client.publish("rainfall", json.dumps({"rainfall": n})).wait_for_publish(timeout=5)
            client.publish("temperature", str(n)).wait_for_publish(timeout=5)
            time.sleep(1)
    except KeyboardInterrupt:
        pass

    client.disconnect()


if __name__ == "__main__":
    main()
