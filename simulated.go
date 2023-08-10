package traefik_create_simulated

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Config the plugin configuration.
type Config struct {
	IotHubUrl       string
	SubscriptionKey string
}

func CreateConfig() *Config {
	return &Config{}
}

type SimulatedPlugin struct {
	next            http.Handler
	client          *http.Client
	iotHubUrl       string
	subscriptionKey string
}

type CreateThingRequest struct {
	DeviceLinkOperation DeviceLinkOperation `json:"deviceLinkOperation"`
}

type DeviceLinkOperation struct {
	HardwareId HardwareId `json:"identifier"`
	Product    Product    `json:"product"`
}

type CreateSimulatedDeviceRequest struct {
	HardwareId    HardwareId    `json:"hardwareId"`
	Product       Product       `json:"productId"`
	SimulatorType SimulatorType `json:"simulatorType"`
}

type HardwareId string

type Product string

const (
	ProductTracker Product = "TRACKER"
)

type SimulatorType string

const (
	SimulatorTypeManual SimulatorType = "MANUAL"
)

// LogEvent contains a single log entry
type LogEvent struct {
	Level   string    `json:"level"`
	Msg     string    `json:"msg"`
	Time    time.Time `json:"time"`
	Network Network   `json:"network"`
	URL     string    `json:"url"`
}

type Network struct {
	Client `json:"client"`
}

type Client struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// New creates a new plugin
func New(_ context.Context, next http.Handler, config *Config, _ string) (http.Handler, error) {
	simulatedPlugin := &SimulatedPlugin{
		next: next,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		iotHubUrl:       config.IotHubUrl,
		subscriptionKey: config.SubscriptionKey,
	}

	return simulatedPlugin, nil
}

func (plugin *SimulatedPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logError("error reading body: %v", err)
		http.NotFound(w, r)
		return
	}

	d := json.NewDecoder(bytes.NewReader(body))
	cr := &CreateThingRequest{}
	if err := d.Decode(cr); err != nil {
		logError("error decoding body: %v", err)
		http.NotFound(w, r)
		return
	}

	logWarn("found deviceId=%s", cr.DeviceLinkOperation.HardwareId)
	url, err := url.Parse(plugin.iotHubUrl + "/simulator/simulated/device")
	if err != nil {
		logError("error creating url: %v", err)
		http.NotFound(w, r)
		return
	}

	csdr := CreateSimulatedDeviceRequest{
		HardwareId:    HardwareId(cr.DeviceLinkOperation.HardwareId),
		Product:       cr.DeviceLinkOperation.Product,
		SimulatorType: SimulatorTypeManual,
	}

	var csdrJson bytes.Buffer
	e := json.NewEncoder(&csdrJson)
	if err := e.Encode(csdr); err != nil {
		logError("error encoding create simulated device request: %v", err)
		http.NotFound(w, r)
		return
	}

	req, err := http.NewRequest("POST", url.String(), &csdrJson)
	if err != nil {
		logError("error encoding create simulated device request: %v", err)
		http.NotFound(w, r)
		return
	}
	for h, v := range r.Header {
		for _, sv := range v {
			req.Header.Add(h, sv)
		}
	}
	req.Header.Set("X-Subscription-Key", plugin.subscriptionKey)

	resp, err := plugin.client.Do(req)
	if err != nil {
		logError("error performing request to iothub: %v", err)
		http.NotFound(w, r)
		return
	}
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		logError("iot hub status code error: %s", resp.Status)
		http.NotFound(w, r)
		return
	}
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		logError("error reading iot hub response: %v", err)
		http.NotFound(w, r)
		return
	}
	logError("iot hub device created: %s", rb)

	r.Body = NoOpCloser(bytes.NewReader(body))
	plugin.next.ServeHTTP(w, r)
}

func NoOpCloser(r io.Reader) io.ReadCloser {
	return noopCloser{r: r}
}

type noopCloser struct {
	r io.Reader
}

func (n noopCloser) Read(b []byte) (int, error) { return n.r.Read(b) }
func (n noopCloser) Close() error               { return nil }

func logInfo(format string, v ...any) *LogEvent {
	return newLogEvent("info", fmt.Sprintf(format, v...))
}

func logWarn(format string, v ...any) *LogEvent {
	return newLogEvent("warn", fmt.Sprintf(format, v...))
}

func logError(format string, v ...any) *LogEvent {
	return newLogEvent("error", fmt.Sprintf(format, v...))
}

func newLogEvent(level, msg string) *LogEvent {
	return &LogEvent{
		Level: level,
		Msg:   msg,
	}
}

func (logEvent *LogEvent) print() {
	jsonLogEvent, _ := json.Marshal(*logEvent)
	fmt.Println(string(jsonLogEvent))
}

func (logEvent *LogEvent) withNetwork(network Network) *LogEvent {
	logEvent.Network = network
	return logEvent
}

func (logEvent *LogEvent) withUrl(url string) *LogEvent {
	logEvent.URL = url
	return logEvent
}
