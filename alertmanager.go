package gologger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

var alertQueue *AlertQueue

// WithAlertManager sets up the logger to send alerts to an alertmanager instance
// the context is used to check if the alertmanager instance is reachable AND for the runtime
// if the context is cancelled, the alertmanager will stop sending alerts
// alertmanagerHost is the host of the alertmanager instance
// baseLabels are the labels that will be added to all alerts, e.g. {"instance": "my-service"}
// returns an error if the alertmanager instance is not reachable
func WithAlertManager(ctx context.Context, alertmanagerHost string, instance, service string, baseLabels map[string]string) error {
	if baseLabels == nil {
		baseLabels = make(map[string]string)
	}
	if alertmanagerHost == "" {
		return fmt.Errorf("alertmanagerHost must be set")
	}
	if instance == "" {
		return fmt.Errorf("instance must be set")
	}
	if service == "" {
		return fmt.Errorf("service must be set")
	}
	baseLabels["instance"] = instance
	baseLabels["service"] = service

	err := waitForAlertmanager(context.Background(), alertmanagerHost)
	if err != nil {
		return err
	}

	if alertQueue != nil {
		return fmt.Errorf("alertmanager already set up")
	}
	alertQueue = &AlertQueue{
		host:       alertmanagerHost,
		baseLabels: baseLabels,
		batchWait:  5 * time.Second,
		batch:      make(chan aalert),
	}
	go alertQueue.run(ctx)

	return nil
}

type aalert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	GeneratorURL string            `json:"generatorURL"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
}

// NewAlert creates a new alert with the given parameters
// name is the name of the alert. If it is empty, an error will be returned.
// summary is a short description of the alert. If it is empty, an error will be returned.
// startsAt is the time when the alert starts, endsAt is the time when the alert ends. If they are omitted the current time will be used for both. If only one is omitted, the other will be set to it as well. If startsAt is after endsAt an error will be returned.
// labels are necessary information that will be sent to the alertmanager. The name, instance and baseLabels will be added to the annotations and overwrite existing keys.
// annotations are additional information that will be sent to the alertmanager. The summary will be added to the annotations. A common and recommended annotation is "summary" with a short description of the alert.
// generatorURL is the URL of the service that sends the alert. It is optional.
func Alert(name string, summary string, startsAt, endsAt time.Time, labels, annotations map[string]string, generatorURL string) error {
	if alertQueue == nil {
		return fmt.Errorf("alertmanager not not intialized. Use WithAlertManager to initialize it")
	}

	if labels == nil {
		labels = make(map[string]string)
	}
	if name == "" {
		return fmt.Errorf("name must be set")
	}
	labels["alertname"] = name

	if annotations == nil {
		annotations = make(map[string]string)
	}
	if summary == "" {
		return fmt.Errorf("summary must be set")
	}
	annotations["summary"] = summary

	if startsAt.IsZero() {
		if endsAt.IsZero() {
			startsAt = time.Now()
		} else {
			startsAt = endsAt
		}
	}
	if endsAt.IsZero() {
		endsAt = startsAt
	}
	if startsAt.After(endsAt) {
		return fmt.Errorf("startsAt must be before endsAt")
	}

	alert := aalert{
		StartsAt:     startsAt,
		EndsAt:       endsAt,
		Labels:       labels,
		Annotations:  annotations,
		GeneratorURL: generatorURL,
	}

	alertQueue.batch <- alert
	return nil
}

// String implements the fmt.Stringer interface to be able to print a concise representation of the alert
func (a aalert) String() string {
	// Extracting alertname and instance as they are crucial identifiers
	alertName := a.Labels["alertname"]
	instance := a.Labels["instance"]

	// Summary is optional but very useful for quick insights
	summary := a.Annotations["summary"]

	// Creating a concise string representation
	parts := []string{fmt.Sprintf("Alertname: %s", alertName), fmt.Sprintf("Instance: %s", instance)}
	if summary != "" {
		parts = append(parts, fmt.Sprintf("Summary: %s", summary))
	}

	return strings.Join(parts, ", ")
}

type AlertQueue struct {
	host       string
	baseLabels map[string]string

	batchWait time.Duration
	batch     chan aalert
}

func (a *AlertQueue) run(ctx context.Context) {
	currentBatch := make([]aalert, 0)
	sendAlerts := func() {
		if len(currentBatch) == 0 {
			return
		}

		// inject baseLabels
		for _, alert := range currentBatch {
			for k, v := range a.baseLabels {
				alert.Labels[k] = v
			}
		}

		err := a.send(currentBatch)
		if err != nil {
			alertString := strings.Join(mapSlice(currentBatch, func(a aalert) string { return a.String() }), ", ")
			// we use std logger here because we don't want to create a potential loop, e.g. if someone hooks to Error logs and sends them as an alert
			slog.Error("failed to send batch to alertmanager", "alertmanagerHost", a.host, "err", err, "alerts", alertString)
		}
		clear(currentBatch)
	}

	ticker := time.NewTicker(a.batchWait)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// send remaining alerts
			sendAlerts()
			return
		case alert := <-a.batch:
			currentBatch = append(currentBatch, alert)
		case <-ticker.C:
			sendAlerts()
		}
	}
}

func (a *AlertQueue) send(alerts []aalert) error {
	// https://github.com/prometheus/alertmanager/blob/main/api/v2/openapi.yaml

	jsonData, err := json.Marshal(alerts)
	if err != nil {
		return fmt.Errorf("failed to marshal alerts: %w", err)
	}

	compressed, err := zip(jsonData)
	if err != nil {
		return fmt.Errorf("failed to compress alerts: %w", err)
	}

	req, err := http.NewRequest("POST", joinUrl(a.host, "/api/v2/alerts"), compressed)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send alerts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("alertmanager responded with unexpected status code: %s - %s", resp.Status, string(body))
	}

	return nil
}

func waitForAlertmanager(ctx context.Context, alertmanagerHost string) error {
	attempts := 0
	for {
		if attempts > 5 {
			return fmt.Errorf("could not connect to loki")
		}
		res, err := http.Get(joinUrl(alertmanagerHost, "/ready"))
		if err != nil {
			attempts += 1
			continue
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			attempts += 1
			time.Sleep(5 * time.Second)
			continue
		}

		return nil
	}
}
