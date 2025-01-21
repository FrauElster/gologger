# gologger

My small logger library. This is not meant to be used by anyone, I just kept copy pasting this in my projects and thought it is time to have it versioned as dependency.

It has a global extensible logger, featuring 2 extensions:

- `console`, basically just standard slog
- `loki`, pushing logs in batches to a Loki instance# gologger


## Usage

```go
// with loki
if err := gologger.Setup(conf.Level); err != nil {
	return err
}

if lokiURL != "" {
	err := gologger.UseLoki(gologger.LokiConfig{
		URL:       lokiURL,
		BatchWait: 5*time.Second,
		Labels: map[string]string{
			"source":  "myapp",
			"version": "1.0",
		},
	})
	if err != nil {
		return err
	}
}

if logFile != "" {
	err := logger.UseFile(gologger.FileConfig{Path: logFile, LabelsMap: map[string]string{"source": "myApp", "version": "1.0"}})
	if err != nil {
		return err
	}
}

gologger.Info("not logged at all")
gologger.Warn("not sent to loki, but printed to stdout", "key", "value")
gologger.Error("sent to loki and stdout")
```
